package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	protocol "sinan/packages/plugin-protocol"
)

func uiManifest() Manifest {
	return Manifest{ID: "example.ui", Name: "Fixture", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "declarative"}, Contributes: Contributions{
		Commands: []Command{{ID: "hello", Title: "Hello", Message: "fixture"}},
		UIViews:  []protocol.UIView{{ID: "main", Entry: "pages/index.html", BridgeVersion: protocol.UIBridgeVersion}},
	}}
}

func TestUIViewDeclarations(t *testing.T) {
	m := uiManifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Contributes.UIViews[0].ID = "" },
		func(m *Manifest) { m.Contributes.UIViews[0].ID = strings.Repeat("a", 65) },
		func(m *Manifest) { m.Contributes.UIViews[0].BridgeVersion = "" },
		func(m *Manifest) { m.Contributes.UIViews[0].BridgeVersion = "2" },
		func(m *Manifest) { m.Contributes.UIViews[0].Permissions = []string{"applications.read"} },
		func(m *Manifest) { m.Contributes.UIViews[0].Permissions = []string{"unknown"} },
		func(m *Manifest) { m.Contributes.UIViews = append(m.Contributes.UIViews, m.Contributes.UIViews[0]) },
		func(m *Manifest) { m.Contributes.UIViews = make([]protocol.UIView, 5) },
		func(m *Manifest) { m.Contributes.Commands[0] = Command{ID: "open", Title: "Open", View: "main"} },
	} {
		m := uiManifest()
		mutate(&m)
		if m.Validate() == nil {
			t.Fatal("invalid UI declaration accepted")
		}
	}
	for _, entry := range []string{"", "../index.html", "/index.html", "https://example.test/index.html", "pages//index.html", "pages/./index.html", "pages/../index.html", "pages\\index.html", "index.html?x=1", "%69ndex.html", "index.js", "INDEX.HTML", strings.Repeat("a/", 9) + "index.html", strings.Repeat("x", 257) + ".html"} {
		m := uiManifest()
		m.Contributes.UIViews[0].Entry = entry
		if m.Validate() == nil {
			t.Fatal("invalid entry accepted")
		}
	}
}

func TestUIOnlyDeclarationDoesNotGrantBackendPermissions(t *testing.T) {
	m := uiManifest()
	m.Runtime = Runtime{Kind: "process", Entry: "bin/fixture"}
	m.Contributes.Commands[0].Message = ""
	m.Permissions = []string{"applications.read", "applications.launch"}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	m.Contributes.UIViews[0].Permissions = []string{"applications.launch"}
	if m.Validate() == nil {
		t.Fatal("backend permission granted to UI")
	}
}

func TestLoadUIRejectsUnknownCapabilityFields(t *testing.T) {
	data, err := json.Marshal(uiManifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"network":true`, `"hostAPI":["execute"]`, `"csp":"unsafe"`} {
		modified := strings.Replace(string(data), `"bridgeVersion":"1"`, `"bridgeVersion":"1",`+field, 1)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(modified), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("unknown UI field accepted")
		}
	}
}
