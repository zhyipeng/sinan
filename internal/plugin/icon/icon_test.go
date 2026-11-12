package icon

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const artwork = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><path d="M1 2h3" stroke="#315ed8"/></svg>`

func TestPassiveArtwork(t *testing.T) {
	if value := DataURL([]byte(artwork)); value == "" || Normalize(value) != value {
		t.Fatal("SVG round trip failed")
	}
	var bitmap bytes.Buffer
	if err := png.Encode(&bitmap, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(DataURL(bitmap.Bytes()), "data:image/png;base64,") {
		t.Fatal("PNG not accepted")
	}
	for _, bad := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<svg><image href="https://example.com/pixel"/></svg>`,
		`<svg><path fill="url(https://example.com/pixel)"/></svg>`,
		`<svg onload="alert(1)"/>`, `<svg style="background: red"/>`,
		`<!DOCTYPE svg><svg/>`, `<svg/><svg/>`, `<svg><path></svg>`, `secrets`,
	} {
		if DataURL([]byte(bad)) != "" {
			t.Fatal("unsafe artwork accepted")
		}
	}
	for _, value := range []string{"https://example.com/icon.png", "file:///tmp/icon.png", strings.Repeat("x", MaxBytes*2+1)} {
		if Normalize(value) != "" {
			t.Fatal("non embedded catalog icon accepted")
		}
	}
	if DataURL(bytes.Repeat([]byte("x"), MaxBytes+1)) != "" {
		t.Fatal("oversize accepted")
	}
}

func TestConfinedFilesAndFallback(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "icon.svg"), []byte(artwork), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "icon.svg"), []byte(artwork), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "icon.svg"), filepath.Join(root, "escape.svg")); err != nil {
		t.Fatal(err)
	}
	if Load(root, "icon.svg") == "" {
		t.Fatal("local artwork missing")
	}
	for _, path := range []string{"", "missing.svg", "escape.svg", "../icon.svg", "/tmp/icon.svg", `dir\icon.svg`, "https://example.com/icon.svg", "file.txt"} {
		if Load(root, path) != "" {
			t.Fatalf("expected fallback for %q", path)
		}
	}
}
