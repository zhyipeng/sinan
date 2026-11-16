// Package clipboard owns system clipboard adapters, never history or monitoring.
package clipboard

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"unicode/utf8"

	protocol "sinan/packages/plugin-protocol"
)

const maxText = 256 << 10
const maxImage = 8 << 20

type content = protocol.ClipboardContent

func validate(c content) (int, int, error) {
	if c.Kind == "text" && len(c.Data) > 0 && len(c.Data) <= maxText && utf8.Valid(c.Data) && !bytes.ContainsRune(c.Data, 0) {
		return 0, 0, nil
	}
	if c.Kind == "image" && len(c.Data) > 0 && len(c.Data) <= maxImage {
		cfg, err := png.DecodeConfig(bytes.NewReader(c.Data))
		if err == nil && cfg.Width > 0 && cfg.Height > 0 && int64(cfg.Width)*int64(cfg.Height) <= 16_000_000 {
			return cfg.Width, cfg.Height, nil
		}
	}
	return 0, 0, errors.New("剪贴板内容无效或超出限制")
}

var gate = make(chan struct{}, 1)

// Call serializes access across plugins and checks cancellation before admission.
func Call(ctx context.Context, method string, p protocol.ClipboardParams) (protocol.ClipboardResult, error) {
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return protocol.ClipboardResult{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return protocol.ClipboardResult{}, err
	}
	if method == "clipboard.read" {
		c, sequence, err := readClipboard(p.Sequence)
		if err != nil {
			return protocol.ClipboardResult{}, errors.New(clipboardStatus(err))
		}
		if len(c.Data) > 0 {
			if _, _, err = validate(c); err != nil {
				return protocol.ClipboardResult{}, err
			}
		}
		return protocol.ClipboardResult{Content: c, Sequence: sequence}, nil
	}
	if method != "clipboard.write" {
		return protocol.ClipboardResult{}, errors.New("剪贴板方法无效")
	}
	if _, _, err := validate(p.Content); err != nil {
		return protocol.ClipboardResult{}, err
	}
	if err := writeClipboard(p.Content); err != nil {
		return protocol.ClipboardResult{}, errors.New(clipboardStatus(err))
	}
	return protocol.ClipboardResult{}, nil
}
