package theme

import (
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

func TestBuiltinContractAndSafeProvider(t *testing.T) {
	state := Builtins("system", 0)
	if ValidateState(state) != nil {
		t.Fatal("invalid builtins")
	}
	registry := NewRegistry()
	def := clone(state.Dark)
	def.ID = "example.themes.night"
	def.Name = "Night"
	if err := registry.Register("example.themes", []protocol.ThemeDefinition{def}); err != nil {
		t.Fatal(err)
	}
	def.Tokens["background"] = "#abcdef"
	stored, _ := registry.Get(def.ID)
	if stored.Tokens["background"] == "#abcdef" {
		t.Fatal("provider mutated registered theme")
	}
	stored.Tokens["text"] = "#123456"
	again, _ := registry.Get(def.ID)
	if again.Tokens["text"] == "#123456" {
		t.Fatal("consumer mutated registry")
	}
	for _, value := range []string{"url(https://example.invalid)", "red", "#abc", "#123456; color:red", "var(--host)"} {
		bad := clone(state.Light)
		bad.ID = "other.themes.bad"
		bad.Tokens["background"] = value
		if registry.Register("other.themes", []protocol.ThemeDefinition{bad}) == nil {
			t.Fatal("unsafe or incomplete color accepted")
		}
	}
	good := clone(state.Light)
	good.ID = "new.themes.good"
	bad := clone(good)
	bad.ID = "unrelated.themes.bad"
	if registry.Register("new.themes", []protocol.ThemeDefinition{good, bad}) == nil {
		t.Fatal("foreign namespace accepted")
	}
	if _, ok := registry.Get(good.ID); ok {
		t.Fatal("failed provider batch partially published")
	}
	invalid := clone(state.Light)
	invalid.Tokens["background"] = ""
	if Validate(invalid) == nil {
		t.Fatal("missing token accepted")
	}
	if ValidateState(protocol.ThemeState{Mode: "plugin", Light: state.Light, Dark: state.Dark}) == nil {
		t.Fatal("unsupported mode accepted")
	}
	if strings.Contains(state.Dark.Tokens["background"], "url") {
		t.Fatal("unexpected builtin resource")
	}
}
