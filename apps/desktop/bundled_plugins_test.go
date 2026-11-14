package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"sinan/internal/kernel"
)

func TestBundledFirstRunPreservesExistingPlugins(t *testing.T) {
	manager, err := kernel.New(filepath.Join(t.TempDir(), "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	bundle := fstest.MapFS{}
	for _, id := range []string{"system.app-search", "system.settings", "system.plugin-manager"} {
		bundle["bundled/"+id+"/manifest.json"] = &fstest.MapFile{Data: []byte(fmt.Sprintf(`{"id":%q,"name":"Bundled","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"hello","title":"Hello","message":"ok"}]}}`, id))}
	}
	if err := installMissingPlugins(manager, bundle); err != nil {
		t.Fatal(err)
	}
	if got := len(manager.Snapshot().Plugins); got != 3 {
		t.Fatalf("installed %d plugins", got)
	}
	// An installed package is not overwritten even if shipped bytes change.
	bundle["bundled/system.settings/manifest.json"].Data = []byte("invalid replacement")
	if err := installMissingPlugins(manager, bundle); err != nil {
		t.Fatal(err)
	}
	if got := len(manager.Snapshot().Plugins); got != 3 {
		t.Fatalf("installed %d plugins after restart", got)
	}
}

func TestBundledEmptySourceAndCorruptDestination(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	manager, err := kernel.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := installMissingPlugins(manager, fstest.MapFS{}); err != nil {
		t.Fatal(err)
	}
	// A corrupt destination must not be silently replaced by a first-run install.
	target := filepath.Join(root, "system.settings")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("preserve corrupt local bytes")
	if err := os.WriteFile(filepath.Join(target, "manifest.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	bundle := fstest.MapFS{"bundled/system.settings/manifest.json": &fstest.MapFile{Data: []byte(`{"id":"system.settings","name":"Settings","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"hello","title":"Hello","message":"ok"}]}}`)}}
	bundle["bundled/system.plugin-manager/manifest.json"] = &fstest.MapFile{Data: []byte(`{"id":"system.plugin-manager","name":"Manager","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"hello","title":"Hello","message":"ok"}]}}`)}
	if err := installMissingPlugins(manager, bundle); err == nil {
		t.Fatal("corrupt destination was accepted")
	}
	data, err := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err != nil || string(data) != string(original) {
		t.Fatal("existing bytes changed")
	}
	if plugins := manager.Snapshot().Plugins; len(plugins) != 1 || plugins[0].ID != "system.plugin-manager" {
		t.Fatal("one broken plugin blocked an unrelated bundled plugin")
	}
}

func TestBundledShippedSnapshot(t *testing.T) {
	if _, err := bundledPlugins.ReadFile("bundled/system.app-search/manifest.json"); err != nil {
		t.Skip("裸 Go 测试没有生成发行快照；统一构建入口会生成并验证")
	}
	manager, err := kernel.New(filepath.Join(t.TempDir(), "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := installMissingBundledPlugins(manager); err != nil {
		t.Fatal(err)
	}
	if len(manager.Snapshot().Plugins) != 3 {
		t.Fatal("发行快照未包含三个可安装的内置插件")
	}
}
