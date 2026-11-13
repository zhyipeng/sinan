package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformLinuxReadsLocalizedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor.desktop")
	if err := os.WriteFile(path, []byte("[Desktop Entry]\nType=Application\nName=Editor\nName[zh_CN]=编辑器\nExec=editor %F\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	meta, err := readMetadata(path)
	if err != nil || meta.DisplayName != "编辑器" {
		t.Fatal(meta, err)
	}
}
