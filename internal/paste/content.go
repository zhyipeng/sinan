// Package paste validates explicit clipboard input, without reading or watching
// the OS clipboard. Content is immutable and retained only in memory.
package paste

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"sinan/internal/inputmatch"
	protocol "sinan/packages/plugin-protocol"
	"unicode/utf8"
)

const MaxImageBytes = 4 << 20
const MaxImagePixels = 16 << 20

type Content struct {
	text          *string
	image         []byte
	width, height int
}

func Parse(value protocol.PasteContent) (*Content, error) {
	if (value.Text == nil) == (value.Image == nil) {
		return nil, errors.New("粘贴内容须为一份文本或图片")
	}
	if value.Text != nil {
		text := *value.Text
		if len(text) == 0 || len(text) > inputmatch.MaxInputBytes || !utf8.ValidString(text) {
			return nil, errors.New("粘贴文本为空、无效或超过 64 KiB")
		}
		return &Content{text: &text}, nil
	}
	encoded := value.Image.DataBase64
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxImageBytes) {
		return nil, errors.New("粘贴图片超过 4 MiB")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > MaxImageBytes {
		return nil, errors.New("粘贴图片无效或过大")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > MaxImagePixels/config.Height {
		return nil, errors.New("粘贴图片无效或超过 1600 万像素")
	}
	// Reject truncated or malformed pixel data, not just an apparently valid header.
	if _, err = png.Decode(bytes.NewReader(data)); err != nil {
		return nil, errors.New("粘贴图片无法解码")
	}
	return &Content{image: data, width: config.Width, height: config.Height}, nil
}
func (c *Content) Text() (string, bool) {
	if c == nil || c.text == nil {
		return "", false
	}
	return *c.text, true
}
func (c *Content) Image() ([]byte, int, int) {
	if c == nil {
		return nil, 0, 0
	}
	return bytes.Clone(c.image), c.width, c.height
}
func (c *Content) IsImage() bool { return c != nil && len(c.image) > 0 }
func (c *Content) Matches(rules []protocol.InputMatcher) bool {
	if c.IsImage() {
		for _, r := range rules {
			if r.Kind == "image" {
				return true
			}
		}
		return false
	}
	text, ok := c.Text()
	return ok && inputmatch.NewQuery(text).MatchAny(rules)
}
