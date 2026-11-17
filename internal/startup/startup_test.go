package startup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformStartupFileLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autostart", "sinan")
	r := fileRegistration{path, []byte("registered executable")}
	if enabled, err := r.Enabled(); err != nil || enabled {
		t.Fatalf("default: %v %v", enabled, err)
	}
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := r.Enabled(); err != nil || !enabled {
		t.Fatalf("enable: %v %v", enabled, err)
	}
	// Moving the executable must not report the obsolete registration as enabled.
	if enabled, err := (fileRegistration{path, []byte("new executable")}).Enabled(); err != nil || enabled {
		t.Fatalf("stale path: %v %v", enabled, err)
	}
	if err := r.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := r.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := r.Enabled(); err != nil || enabled {
		t.Fatalf("disable: %v %v", enabled, err)
	}
	blocked := fileRegistration{filepath.Join(path, "child"), r.content}
	if err := os.WriteFile(path, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := blocked.SetEnabled(true); err == nil {
		t.Fatal("write failure accepted")
	}
	if _, err := (fileRegistration{filepath.Dir(path), r.content}).Enabled(); err == nil {
		t.Fatal("read failure accepted")
	}
}

func TestPlatformStartupRejectsInvalidPath(t *testing.T) {
	for _, path := range []string{"relative", "", filepath.Join(t.TempDir(), "bad\npath")} {
		if _, err := New(path); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
}
