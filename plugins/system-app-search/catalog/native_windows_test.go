package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestPlatformWindowsMetadataAndCancelledLaunch(t *testing.T) {
	meta, err := readMetadata(filepath.Join(t.TempDir(), "Text Editor.lnk"))
	if err != nil || meta.Name != "Text Editor" {
		t.Fatal(meta, err)
	}
	if _, err := readMetadata("command.cmd"); err == nil {
		t.Fatal("accepted non-application entry")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := launchApplication(ctx, "never-launched.exe"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
