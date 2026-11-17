package startup

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformStartupDesktopEntry(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	r, err := New("/opt/司南 a\"b$`%\\/sinan")
	if err != nil {
		t.Fatal(err)
	}
	f := r.(fileRegistration)
	if f.path != filepath.Join(config, "autostart", "dev.sinan.desktop.desktop") {
		t.Fatal("incorrect XDG path")
	}
	if !strings.Contains(string(f.content), "Exec=\"/opt/司南 a\\\\\"b\\\\$\\\\`%%\\\\\\\\/sinan\"\n") {
		t.Fatalf("invalid Exec escaping: %q", f.content)
	}
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := r.Enabled(); err != nil || !enabled {
		t.Fatalf("registration: %v %v", enabled, err)
	}
}
