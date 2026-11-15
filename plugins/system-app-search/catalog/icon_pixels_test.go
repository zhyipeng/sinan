package catalog

import (
	"bytes"
	"image/color"
	"testing"
)

func TestPlatformWindowsIconPixels(t *testing.T) {
	black := make([]byte, applicationIconSize*applicationIconSize*4)
	white := bytes.Repeat([]byte{255}, len(black))
	// Transparent, opaque red, and half-transparent red. DIB alpha is
	// intentionally zero: legacy icon rendering does not supply usable alpha.
	copy(black[4:], []byte{0, 0, 255, 0, 0, 0, 128, 0})
	copy(white[4:], []byte{0, 0, 255, 0, 127, 127, 255, 0})
	img := windowsIconPixels(black, white)
	for x, want := range []color.NRGBA{{}, {R: 255, A: 255}, {R: 255, A: 128}} {
		if got := img.NRGBAAt(x, 0); got != want {
			t.Fatalf("pixel %d: got %v, want %v", x, got, want)
		}
	}
}
