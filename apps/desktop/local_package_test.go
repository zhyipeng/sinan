package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sinan/internal/kernel"
	"sinan/internal/paste"
	"sinan/internal/plugin/manifest"
	"sinan/internal/search"
	protocol "sinan/packages/plugin-protocol"
)

func writeLocalZIP(t *testing.T, target, version, kind string) {
	t.Helper()
	mf := manifest.Manifest{ID: "example.local", Name: "Local", Version: version, APIVersion: "1", Runtime: manifest.Runtime{Kind: kind}, Contributes: manifest.Contributions{Commands: []manifest.Command{{ID: "hello", Title: "Hello", Message: "Hello"}}}}
	if kind == "process" {
		mf.Runtime.Entry = "backend"
		mf.Contributes.Commands[0].Message = ""
	}
	data, err := json.Marshal(mf)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(file)
	entry, err := z.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if kind == "process" {
		header := &zip.FileHeader{Name: "backend"}
		header.SetMode(0755)
		entry, err = z.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalZIPInstallUpgradeAndChangedSource(t *testing.T) {
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	host := &HostService{manager: manager}
	source := filepath.Join(t.TempDir(), "plugin.zip")
	writeLocalZIP(t, source, "0.1.0", "declarative")
	preview, err := host.PreviewPackage(source)
	if err != nil {
		t.Fatal(err)
	}
	writeLocalZIP(t, source, "0.2.0", "declarative")
	if _, err = host.ApplyPackage(source, false, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
		t.Fatal("changed archive installed without review")
	}
	preview, err = host.PreviewPackage(source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := host.ApplyPackage(source, false, preview.Fingerprint, preview.InstalledFingerprint)
	if err != nil || installed.Version != "0.2.0" {
		t.Fatalf("install failed: %v", err)
	}
	writeLocalZIP(t, source, "0.3.0", "declarative")
	preview, err = host.PreviewPackage(source)
	if err != nil || preview.InstalledVersion != "0.2.0" {
		t.Fatalf("upgrade preview failed: %v", err)
	}
	installed, err = host.ApplyPackage(source, false, preview.Fingerprint, preview.InstalledFingerprint)
	if err != nil || installed.Version != "0.3.0" {
		t.Fatalf("upgrade failed: %v", err)
	}
}

func TestLocalZIPNativeRequiresTrust(t *testing.T) {
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	host := &HostService{manager: manager}
	source := filepath.Join(t.TempDir(), "native.zip")
	writeLocalZIP(t, source, "0.1.0", "process")
	preview, err := host.PreviewPackage(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = host.ApplyPackage(source, false, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
		t.Fatal("native installed without trust")
	}
	if _, err = host.ApplyPackage(source, true, preview.Fingerprint, preview.InstalledFingerprint); err != nil {
		t.Fatal(err)
	}
}

func TestSearchLocalZIPInstall(t *testing.T) {
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	host := &HostService{manager: manager}
	coordinator := host.searchCoordinator()
	ctx := context.Background()
	var found *search.Item
	receive := func(update search.Update) {
		found = nil
		for _, item := range update.Items {
			if item.ProviderID == "$local-package" {
				found = &item
				break
			}
		}
	}
	dir := filepath.Join(t.TempDir(), "plugin packages")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	const name = "example.local-0.1.0-linux-arm64.zip"
	source := filepath.Join(dir, name)
	writeLocalZIP(t, source, "0.1.0", "declarative")
	coordinator.Query(ctx, "missing-manager", source, receive)
	if found != nil {
		t.Fatal("install result appeared without plugin center")
	}
	if _, err := manager.Install(filepath.Join("..", "..", "plugins", "system-plugin-manager"), false); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		name,
		filepath.Join(dir, "archive.zip"),
		filepath.Join(dir, "invalid-0.1.0-linux-arm64.zip"),
		filepath.Join(dir, "example.local-0.01.0-linux-arm64.zip"),
		filepath.Join(dir, "example.local-0.1.0-linux-x86.zip"),
		source + ".sha256",
		source + string(filepath.Separator),
		source + "\n" + source,
	} {
		coordinator.Query(ctx, "invalid", input, receive)
		if found != nil {
			t.Fatal("invalid package path matched")
		}
	}
	coordinator.Query(ctx, "typed", source, receive)
	if found == nil || found.Title != "安装插件" || found.Subtitle != name || found.Input != source || found.CommandID != "open" {
		t.Fatal("typed path did not yield the install action")
	}
	quoted := " \"" + source + "\" "
	content, err := paste.Parse(protocol.PasteContent{Text: &quoted})
	if err != nil {
		t.Fatal(err)
	}
	coordinator.QueryPaste(ctx, "pasted", "opaque", content, receive)
	if found == nil || found.Input != source {
		t.Fatal("pasted quoted path did not preserve the archive path")
	}
	preview, err := host.PreviewPackage(found.Input)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := host.ApplyPackage(found.Input, false, preview.Fingerprint, preview.InstalledFingerprint)
	if err != nil || installed.ID != "example.local" {
		t.Fatalf("matched ZIP did not install: %v", err)
	}
}
