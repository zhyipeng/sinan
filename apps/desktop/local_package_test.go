package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
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
