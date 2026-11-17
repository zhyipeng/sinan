package manifest

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestInvalidManifest(t *testing.T) {
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.ID = "../escape" },
		func(m *Manifest) { m.APIVersion = "2" },
		func(m *Manifest) { m.Permissions = []string{"process.execute"} },
		func(m *Manifest) { m.Runtime = Runtime{Kind: "process", Entry: "../plugin"} },
		func(m *Manifest) { m.Runtime.Kind = "javascript" },
		func(m *Manifest) { m.Contributes.Commands = append(m.Contributes.Commands, m.Contributes.Commands[0]) },
		func(m *Manifest) { m.ActivationEvents = []string{"onStartup"} },
	} {
		m := Manifest{ID: "example.test", Name: "Test", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{Commands: []Command{{ID: "hello", Title: "Hello", Message: "hello"}}}}
		mutate(&m)
		if m.Validate() == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, data := range []string{`{"id":"example.test","secret":"do not echo"}`, `{} {}`, `not json`} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}

func TestApplicationPermissionsAndExecutionModes(t *testing.T) {
	m, err := Load("../../../plugins/system-app-search")
	if err != nil {
		t.Fatal(err)
	}
	m.Permissions = append(m.Permissions, "applications.read")
	if m.Validate() == nil {
		t.Fatal("duplicate permission accepted")
	}
	m.Permissions = nil
	m.Contributes.Commands[0].Execution = "shell"
	if m.Validate() == nil {
		t.Fatal("unknown execution mode accepted")
	}
}

func TestBundledViewAndSettingsDeclarations(t *testing.T) {
	m, err := Load("../../../plugins/system-settings")
	if err != nil {
		t.Fatal(err)
	}
	if m.Contributes.Commands[0].View != "settings" {
		t.Fatal("missing view key")
	}
	m.Contributes.Commands[0].Message = "mixed"
	if m.Validate() == nil {
		t.Fatal("view and message mixed")
	}
	m, err = Load("../../../plugins/system-app-search")
	if err != nil {
		t.Fatal(err)
	}
	m.Contributes.Settings = append(m.Contributes.Settings, m.Contributes.Settings[0])
	if m.Validate() == nil {
		t.Fatal("duplicate settings key accepted")
	}
}

func TestInputMatcherDeclarations(t *testing.T) {
	valid := func() Manifest {
		return Manifest{ID: "example.match", Name: "Match", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{
			Commands: []Command{{ID: "open", Title: "Open", UIView: "editor", InputMatchers: []protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object", "array"}}}}},
			UIViews:  []protocol.UIView{{ID: "editor", Entry: "index.html", BridgeVersion: "2"}},
		}}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Contributes.UIViews[0].BridgeVersion = "1" },
		func(m *Manifest) { m.Contributes.Commands[0].SearchOnly = true },
		func(m *Manifest) { m.Contributes.Commands[0].UIView = ""; m.Contributes.Commands[0].View = "trusted" },
		func(m *Manifest) {
			m.Contributes.Commands[0].InputMatchers = []protocol.InputMatcher{{Kind: "regex", Pattern: "(?=x)"}}
		},
		func(m *Manifest) { m.Contributes.Commands[0].InputMatchers = make([]protocol.InputMatcher, 9) },
		func(m *Manifest) {
			base := m.Contributes.Commands[0]
			base.InputMatchers = []protocol.InputMatcher{{Kind: "regex", Pattern: "a"}, {Kind: "regex", Pattern: "b"}, {Kind: "regex", Pattern: "c"}, {Kind: "regex", Pattern: "d"}, {Kind: "regex", Pattern: "e"}, {Kind: "regex", Pattern: "f"}, {Kind: "regex", Pattern: "g"}, {Kind: "regex", Pattern: "h"}}
			for i := range 5 {
				c := base
				c.ID = fmt.Sprintf("open-%d", i)
				m.Contributes.Commands = append(m.Contributes.Commands, c)
			}
		},
	} {
		m := valid()
		mutate(&m)
		if m.Validate() == nil {
			t.Fatal("invalid input matching declaration accepted")
		}
	}
	// The same declaration works for native commands, without a JSON-specific path.
	m := valid()
	m.Runtime = Runtime{Kind: "process", Entry: "backend/plugin"}
	m.Contributes.Commands[0].UIView = ""
	m.Contributes.Commands[0].Execution = "direct"
	m.Contributes.Commands[0].InputMatchers = []protocol.InputMatcher{{Kind: "regex", Pattern: `^https?://\S+$`}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestImageMatcherRequiresHTMLBridge2(t *testing.T) {
	m := Manifest{ID: "example.image", Name: "Image", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{
		Commands: []Command{{ID: "open", Title: "Open", UIView: "editor", InputMatchers: []protocol.InputMatcher{{Kind: "image"}}}}, UIViews: []protocol.UIView{{ID: "editor", Entry: "index.html", BridgeVersion: "2"}},
	}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Contributes.UIViews[0].BridgeVersion = "1"
	if m.Validate() == nil {
		t.Fatal("bridge 1 accepted images")
	}
	m.Contributes.UIViews[0].BridgeVersion = "2"
	m.Contributes.Commands[0].UIView = ""
	m.Contributes.Commands[0].Message = "message"
	if m.Validate() == nil {
		t.Fatal("message command accepted image input")
	}
}

func TestPlatformExecutableEntry(t *testing.T) {
	for _, tc := range []struct {
		platform, path string
		mode           os.FileMode
		valid          bool
	}{
		{"windows", "plugin.exe", 0600, true},
		{"windows", "PLUGIN.EXE", 0600, true},
		{"windows", "plugin.cmd", 0700, false},
		{"windows", "plugin", 0700, false},
		{"linux", "plugin", 0700, true},
		{"linux", "plugin", 0600, false},
		{"darwin", "plugin", 0700, true},
	} {
		if executableEntry(tc.platform, tc.path, tc.mode) != tc.valid {
			t.Fatal("incorrect platform executable policy")
		}
	}
}

func TestSettingTypes(t *testing.T) {
	base := func() Manifest {
		return Manifest{ID: "example.test", Name: "Test", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{Commands: []Command{{ID: "hello", Title: "Hello", Message: "hello"}}}}
	}
	valid := base()
	valid.Contributes.Settings = []Setting{
		{ID: "mirror", Title: "镜像", Type: "select", Options: []SettingOption{{Value: "auto", Label: "自动"}, {Value: "official", Label: "仅官方"}}},
		{ID: "template", Title: "模板", Type: "text", Placeholder: "https://x/{url}"},
		{ID: "directories", Title: "目录", Type: "directory-list"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Manifest){
		"select-without-options": func(m *Manifest) { m.Contributes.Settings = []Setting{{ID: "mirror", Title: "镜像", Type: "select"}} },
		"select-bad-value": func(m *Manifest) {
			m.Contributes.Settings = []Setting{{ID: "mirror", Title: "镜像", Type: "select", Options: []SettingOption{{Value: "Auto", Label: "x"}}}}
		},
		"select-dup-option": func(m *Manifest) {
			m.Contributes.Settings = []Setting{{ID: "mirror", Title: "镜像", Type: "select", Options: []SettingOption{{Value: "a", Label: "a"}, {Value: "a", Label: "b"}}}}
		},
		"directory-list-options": func(m *Manifest) {
			m.Contributes.Settings = []Setting{{ID: "directories", Title: "目录", Type: "directory-list", Options: []SettingOption{{Value: "a", Label: "a"}}}}
		},
		"text-options": func(m *Manifest) {
			m.Contributes.Settings = []Setting{{ID: "t", Title: "t", Type: "text", Options: []SettingOption{{Value: "a", Label: "a"}}}}
		},
		"unknown-type": func(m *Manifest) { m.Contributes.Settings = []Setting{{ID: "t", Title: "t", Type: "unknown"}} },
	} {
		m := base()
		mutate(&m)
		if m.Validate() == nil {
			t.Fatalf("invalid settings accepted: %s", name)
		}
	}
}

func TestInputMatchScore(t *testing.T) {
	makeManifest := func(score float64) Manifest {
		return Manifest{ID: "example.rank", Name: "Rank", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{Commands: []Command{{ID: "open", Title: "Open", Message: "ok", InputMatchers: []protocol.InputMatcher{{Kind: "regex", Pattern: `\S`}}, InputMatchScore: &score}}}}
	}
	for _, score := range []float64{0, .2, .95} {
		m := makeManifest(score)
		if err := m.Validate(); err != nil {
			t.Fatal(err)
		}
		if m.Contributes.Commands[0].ContentMatchScore() != score {
			t.Fatal("score not preserved")
		}
	}
	for _, score := range []float64{-.01, .951, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if makeManifest(score).Validate() == nil {
			t.Fatal("invalid score accepted")
		}
	}
	m := makeManifest(.2)
	m.Contributes.Commands[0].InputMatchers = nil
	if m.Validate() == nil {
		t.Fatal("score without matcher accepted")
	}
	m.Contributes.Commands[0].InputMatchScore = nil
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.Contributes.Commands[0].ContentMatchScore() != .95 {
		t.Fatal("legacy score changed")
	}
}

func TestHiddenCommandsRequireScopedWindowExecution(t *testing.T) {
	valid := func() Manifest {
		return Manifest{ID: "example.capture", Name: "Capture", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "process", Entry: "backend/plugin"}, Contributes: Contributions{Commands: []Command{{ID: "capture", Title: "Capture", UIView: "editor", StartHidden: true, Shortcut: "CmdOrCtrl+Shift+A"}, {ID: "region", Title: "Region", SearchOnly: true}}, UIViews: []protocol.UIView{{ID: "editor", Entry: "index.html", BridgeVersion: "2", Commands: []string{"region"}, Permissions: []string{"window.execute"}}}}, ActivationEvents: []string{"onCommand:capture"}}
	}
	mf := valid()
	mf.Contributes.UIViews[0].Permissions = []string{"window.execute", "window.show", "window.hide", "file.savePNG", "file.openImage", "clipboard.read"}
	if err := mf.Validate(); err != nil {
		t.Fatal("six separately declared image permissions rejected", err)
	}
	mf.Contributes.UIViews[0].Permissions = append(mf.Contributes.UIViews[0].Permissions, "storage")
	if mf.Validate() == nil {
		t.Fatal("permission declaration limit ignored")
	}
	mf = valid()
	if err := mf.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){func(m *Manifest) { m.Contributes.UIViews[0].Permissions = nil }, func(m *Manifest) { m.Contributes.UIViews[0].BridgeVersion = "1" }, func(m *Manifest) { m.Contributes.Commands[0].UIView = "" }, func(m *Manifest) { m.Contributes.Commands[0].SearchOnly = true }, func(m *Manifest) { m.Permissions = []string{"window.execute"} }} {
		mf = valid()
		mutate(&mf)
		if mf.Validate() == nil {
			t.Fatal("invalid hidden command accepted")
		}
	}
}
