package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentNativeTrustReloadAndUnlink(t *testing.T) {
	m := manager(t)
	src := developmentNativeSource(t)
	link := filepath.Join(m.root, "example.native")
	if err := os.Symlink(src, link); err != nil {
		t.Skip("symlink unavailable")
	}
	snapshot := m.Refresh()
	if len(snapshot.Plugins) != 1 {
		t.Fatal(snapshot.Diagnostics)
	}
	info := snapshot.Plugins[0]
	if !info.Development || !info.NeedsTrust {
		t.Fatal("native developer trust bypassed")
	}
	if _, err := m.Execute(context.Background(), info.ID, "echo", "x"); err == nil {
		t.Fatal("untrusted linked native ran")
	}
	if _, err := m.TrustDevelopmentPlugin(info.ID, "wrong"); err == nil {
		t.Fatal("stale approval accepted")
	}
	// Static reload works even before trust; it must not execute the native backend.
	info, err := m.Reload(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.TrustDevelopmentPlugin(info.ID, info.DevelopmentFingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Execute(context.Background(), info.ID, "echo", "approved"); err != nil {
		t.Fatal(err)
	}
	cache, err := m.Execute(context.Background(), info.ID, "echo", "cacheDir")
	if err != nil || cache.Text != filepath.Join(m.root, ".sinan-state", "cache", info.ID) {
		t.Fatal("development cache escaped host state", err)
	}
	if _, err = m.Upgrade(src, true); err == nil {
		t.Fatal("upgrade attempted to rename external source")
	}
	if err = os.WriteFile(filepath.Join(src, "extra.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if !m.Refresh().Plugins[0].Changed {
		t.Fatal("rebuild not detected")
	}
	if _, err = m.Reload(info.ID); err != nil {
		t.Fatal("trusted path reload failed", err)
	}
	if err = m.Uninstall(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(src, "manifest.json")); err != nil {
		t.Fatal("uninstall removed source")
	}
	if _, err = os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("link remains")
	}
}

func TestDevelopmentTrustCannotMoveToAnotherTarget(t *testing.T) {
	root := t.TempDir()
	src := developmentNativeSource(t)
	other := developmentNativeSource(t)
	link := filepath.Join(root, "example.native")
	if err := os.Symlink(src, link); err != nil {
		t.Skip("symlink unavailable")
	}
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	info := m.Snapshot().Plugins[0]
	if _, err = m.TrustDevelopmentPlugin(info.ID, info.DevelopmentFingerprint); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if !m.Refresh().Plugins[0].Changed {
		t.Fatal("retarget not detected")
	}
	if _, err = m.Reload(info.ID); err == nil {
		t.Fatal("retarget reused registration")
	}
	m.Close()
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.Snapshot().Plugins[0].NeedsTrust {
		t.Fatal("trust migrated across paths after restart")
	}
	if _, err = reopened.Execute(context.Background(), info.ID, "echo", "x"); err == nil {
		t.Fatal("retarget ran without trust")
	}
}

func TestUnregisteredDevelopmentSymlinkCannotExposeInternalLinks(t *testing.T) {
	m := manager(t)
	src := source(t, false)
	if err := os.Symlink(t.TempDir(), filepath.Join(src, "nested")); err != nil {
		t.Skip("symlink unavailable")
	}
	if err := os.Symlink(src, filepath.Join(m.root, "example.hello")); err != nil {
		t.Fatal(err)
	}
	if len(m.Refresh().Plugins) != 0 {
		t.Fatal("package-internal symlink accepted")
	}
}

func developmentNativeSource(t *testing.T) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "example.native")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyDirectory(source(t, true), dest); err != nil {
		t.Fatal(err)
	}
	return dest
}
