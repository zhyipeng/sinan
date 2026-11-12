package manifest

import (
	"fmt"
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
