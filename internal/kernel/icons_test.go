package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sinan/internal/plugin/icon"
	"sinan/internal/plugin/manifest"
)

func TestBundledPluginArtwork(t *testing.T) {
	for _, name := range []string{"system-settings", "system-plugin-manager", "system-app-search"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("../../plugins", name)
			mf, err := manifest.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if mf.Icon == "" || icon.Load(dir, mf.Icon) == "" {
				t.Fatal("bundled icon missing or rejected")
			}
			p := &plugin{manifest: mf, dir: dir}
			if !strings.HasPrefix(p.info().Icon, "data:image/svg+xml;base64,") {
				t.Fatal("plugin info lost artwork")
			}
		})
	}
}

func TestPluginInfoMissingIconFallback(t *testing.T) {
	dir := t.TempDir()
	p := &plugin{manifest: manifest.Manifest{Icon: "missing.svg"}, dir: dir}
	if p.info().Icon != "" {
		t.Fatal("missing icon should fall back")
	}
	if err := os.WriteFile(filepath.Join(dir, "missing.svg"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if p.info().Icon != "" {
		t.Fatal("malformed icon should fall back")
	}
}
