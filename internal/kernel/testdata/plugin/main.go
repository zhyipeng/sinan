package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
)

func main() {
	cwd, _ := os.Getwd()
	if _, err := os.Stat("slow-start"); err == nil {
		time.Sleep(time.Second)
	}
	var active atomic.Int32
	_ = sdk.ServeWithHandlers(os.Stdin, os.Stdout, filepath.Base(cwd), sdk.Handlers{
		Command: func(ctx context.Context, p protocol.CommandParams) (protocol.CommandResult, error) {
			switch p.CommandID {
			case "echo":
				if p.Input == "pid" {
					return protocol.CommandResult{Text: strconv.Itoa(os.Getpid())}, nil
				}
				if p.Input == "active" {
					return protocol.CommandResult{Text: strconv.Itoa(int(active.Load()))}, nil
				}
				if p.Input == "delay" {
					time.Sleep(200 * time.Millisecond)
				}
				return protocol.CommandResult{Text: p.Input}, nil
			case "hang":
				time.Sleep(time.Minute)
			case "crash":
				os.Exit(42)
			case "invalid":
				_, _ = os.Stdout.WriteString("not json\n")
				select {}
			}
			return protocol.CommandResult{}, errors.New("unknown command")
		},
		Search: func(ctx context.Context, p protocol.SearchParams) (protocol.SearchResult, error) {
			active.Add(1)
			defer active.Add(-1)
			if p.Query == "ignore" {
				time.Sleep(time.Second)
			}
			if p.Query == "slow" || p.Query == "block" {
				select {
				case <-ctx.Done():
					return protocol.SearchResult{}, ctx.Err()
				case <-time.After(time.Second):
				}
			}
			if p.Query == "crash" {
				os.Exit(42)
			}
			return protocol.SearchResult{Items: []protocol.SearchItem{{ID: "same", Title: p.Query, CommandID: "echo", Input: p.Query, Score: .5}}}, nil
		},
	})
}
