package manifest

import (
	"os"
	"path/filepath"
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
