package paste

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

func pngInput(t *testing.T) protocol.PasteContent {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	return protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(buf.Bytes())}}
}
func TestPasteValidationAndFrozenImage(t *testing.T) {
	value := pngInput(t)
	content, err := Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if !content.IsImage() || !content.Matches([]protocol.InputMatcher{{Kind: "image"}}) || content.Matches([]protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object"}}}) {
		t.Fatal("image matched wrong rules")
	}
	data, w, h := content.Image()
	if w != 2 || h != 3 {
		t.Fatal("wrong image dimensions")
	}
	data[0] = 0
	original, _, _ := content.Image()
	if original[0] == 0 {
		t.Fatal("image exposed mutable bytes")
	}
	text := " \n {} \t"
	c, err := Parse(protocol.PasteContent{Text: &text})
	if err != nil {
		t.Fatal(err)
	}
	text = "changed"
	got, ok := c.Text()
	if !ok || got != " \n {} \t" {
		t.Fatal("text was not captured immutably")
	}
	if !c.Matches([]protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object"}}}) {
		t.Fatal("valid pasted JSON did not match")
	}
}
func TestRejectMalformedOrOversizedPaste(t *testing.T) {
	empty := ""
	large := strings.Repeat("x", (64<<10)+1)
	invalid := string([]byte{0xff})
	both := pngInput(t)
	text := "x"
	both.Text = &text
	image := pngInput(t)
	decoded, _ := base64.StdEncoding.DecodeString(image.Image.DataBase64)
	for _, value := range []protocol.PasteContent{
		{}, both, {Text: &empty}, {Text: &large}, {Text: &invalid},
		{Image: &protocol.PasteImage{DataBase64: "invalid"}},
		{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString([]byte("<svg></svg>"))}},
		{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(decoded[:len(decoded)/2])}},
		{Image: &protocol.PasteImage{DataBase64: strings.Repeat("a", base64.StdEncoding.EncodedLen(MaxImageBytes)+4)}},
	} {
		if _, err := Parse(value); err == nil {
			t.Fatal("invalid paste accepted")
		}
	}
}
