package clipboard

import (
	"bytes"
	"context"
	"errors"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestRejectInvalidAndCancelledWithoutSystemAccess(t *testing.T) {
	for _, c := range []content{{Kind: "text", Data: []byte{0}}, {Kind: "text", Data: []byte{255}}, {Kind: "text", Data: bytes.Repeat([]byte("a"), maxText+1)}, {Kind: "image", Data: []byte("not png")}, {Kind: "file", Data: []byte("fixture")}} {
		if _, err := Call(context.Background(), "clipboard.write", protocol.ClipboardParams{Content: c}); err == nil {
			t.Fatal("invalid system write admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Call(ctx, "clipboard.read", protocol.ClipboardParams{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled read admitted")
	}
}
