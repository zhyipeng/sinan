package manifest

import (
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestDependencyEntryBoundaries(t *testing.T) {
	valid := func() Manifest {
		return Manifest{ID: "example.source", Name: "Source", Version: "0.1.0", APIVersion: "1", Runtime: Runtime{Kind: "process", Entry: "backend/plugin"}, Contributes: Contributions{Commands: []Command{{ID: "open", Title: "Open", UIView: "editor", InputMatchers: []protocol.InputMatcher{{Kind: "image"}}}}, UIViews: []protocol.UIView{{ID: "editor", Entry: "index.html", BridgeVersion: "2", Dependencies: []string{"ocr"}}}, EntryPoints: []protocol.EntryPoint{{ID: "recognize-image", Version: "1", CommandID: "open", Input: "image"}}}, Dependencies: []protocol.Dependency{{ID: "ocr", PluginID: "example.ocr", MinVersion: "0.3.0", Optional: true, Entries: []protocol.EntryRequirement{{ID: "recognize-image", Version: "1"}}}}}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Manifest){func(m *Manifest) { m.Dependencies[0].PluginID = m.ID }, func(m *Manifest) { m.Contributes.UIViews[0].Dependencies = []string{"unlisted"} }, func(m *Manifest) { m.Contributes.UIViews[0].BridgeVersion = "1" }, func(m *Manifest) { m.Contributes.EntryPoints[0].CommandID = "private" }, func(m *Manifest) { m.Contributes.Commands[0].SearchOnly = true }, func(m *Manifest) { m.Dependencies[0].Entries[0].Version = "2" }, func(m *Manifest) { m.Contributes.EntryPoints[0].Input = "path" }} {
		mf := valid()
		change(&mf)
		if mf.Validate() == nil {
			t.Fatal("invalid dependency/export admitted")
		}
	}
	if CompareVersion("0.10.0", "0.3.0") <= 0 || CompareVersion("1.0.0", "0.99.99") <= 0 || CompareVersion("0.3.0", "0.3.0") != 0 {
		t.Fatal("version comparison is lexical")
	}
}
