package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"sinan/internal/paste"
)

func TestImportNormalizesLocalImageFormats(t *testing.T) {
	input := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			var source bytes.Buffer
			var err error
			switch format {
			case "png":
				err = png.Encode(&source, input)
			case "jpeg":
				err = jpeg.Encode(&source, input, nil)
			case "gif":
				err = gif.Encode(&source, input, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			output, w, h, err := normalizeImportImage(source.Bytes())
			if err != nil || w != 8 || h != 6 {
				t.Fatal(w, h, err)
			}
			decoded, err := png.Decode(bytes.NewReader(output))
			if err != nil || decoded.Bounds().Dx() != 8 || decoded.Bounds().Dy() != 6 {
				t.Fatal(err)
			}
		})
	}
}
func TestImportRejectsCorruptOrOversizedImages(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	oversized := bytes.Clone(source.Bytes())
	binary.BigEndian.PutUint32(oversized[16:20], uint32(paste.MaxImagePixels+1))
	binary.BigEndian.PutUint32(oversized[20:24], 1)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, input := range [][]byte{nil, []byte("not image"), source.Bytes()[:source.Len()-8], make([]byte, (8<<20)+1), oversized} {
		if _, _, _, err := normalizeImportImage(input); err == nil {
			t.Fatal("invalid local image accepted")
		}
	}
	var limited imageBuffer
	if _, err := limited.Write(make([]byte, paste.MaxImageBytes)); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Write([]byte{1}); err == nil || limited.Len() != paste.MaxImageBytes {
		t.Fatal("encoded PNG budget exceeded")
	}
}
