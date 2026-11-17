package startup

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestPlatformStartupLaunchAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r, err := New(`/Applications/司南 & tools.app/Contents/MacOS/sinan`)
	if err != nil {
		t.Fatal(err)
	}
	f := r.(fileRegistration)
	var doc struct{ XMLName xml.Name }
	if xml.Unmarshal(f.content, &doc) != nil || doc.XMLName.Local != "plist" {
		t.Fatal("invalid plist")
	}
	if !strings.Contains(string(f.content), "司南 &amp; tools.app") || !strings.Contains(string(f.content), "<key>RunAtLoad</key><true/>") {
		t.Fatal("missing escaped command or login flag")
	}
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := r.Enabled(); err != nil || !enabled {
		t.Fatalf("registration: %v %v", enabled, err)
	}
}
