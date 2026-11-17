package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestCollectPNGChecksChunksAndImageBeforeDialog(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	reply := func(size int, chunk []byte) json.RawMessage {
		text, _ := json.Marshal(map[string]any{"size": size, "data": base64.StdEncoding.EncodeToString(chunk)})
		raw, _ := json.Marshal(map[string]any{"text": string(text)})
		return raw
	}
	id := strings.Repeat("a", 32)
	for _, kind := range []string{"valid", "empty", "bad-png", "truncated", "oversize", "changed-size", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			calls := 0
			got, err := collectPNG(ctx, id, func(_ context.Context, input string) (json.RawMessage, error) {
				calls++
				if kind == "empty" {
					return reply(len(data), nil), nil
				}
				if kind == "bad-png" {
					return reply(3, []byte("bad")), nil
				}
				if kind == "truncated" {
					return reply(len(data)-8, data[:len(data)-8]), nil
				}
				if kind == "oversize" {
					return reply(8<<20+1, data), nil
				}
				if calls == 1 {
					if input != id+":0" {
						t.Fatal(input)
					}
					return reply(len(data), data[:10]), nil
				}
				if kind == "changed-size" {
					return reply(len(data)+1, data[10:]), nil
				}
				if input != id+":10" {
					t.Fatal(input)
				}
				return reply(len(data), data[10:]), nil
			})
			if kind == "valid" {
				if err != nil || !bytes.Equal(got, data) {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid export accepted")
			}
			if kind == "cancelled" && calls != 0 {
				t.Fatal("cancelled export dispatched")
			}
		})
	}
	_, err := collectPNG(context.Background(), "other:0", func(context.Context, string) (json.RawMessage, error) {
		t.Fatal("invalid input dispatched")
		return nil, errors.New("unreachable")
	})
	if err == nil {
		t.Fatal("invalid identifier accepted")
	}
}
