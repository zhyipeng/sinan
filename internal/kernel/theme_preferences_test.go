package kernel

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestThemePreferencesPersistAndStaySeparate(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { m.Close() }()
	state, err := m.ThemeSettings()
	if err != nil || state.Mode != "system" || state.Revision != 0 {
		t.Fatal("bad default", err)
	}
	before, err := m.SaveWindowPreferences(0, "Ctrl+Alt+U")
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.SaveTheme("dark", 0)
	if err != nil || state.Mode != "dark" || state.Revision != 1 {
		t.Fatal("save failed", err)
	}
	if _, err = m.SaveTheme("light", 0); err == nil {
		t.Fatal("stale save accepted")
	}
	if _, err = m.SaveTheme("custom", 1); err == nil {
		t.Fatal("unknown mode accepted")
	}
	current, _ := m.WindowPreferences()
	if !reflect.DeepEqual(current, before) {
		t.Fatal("theme changed window preferences")
	}
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.ThemeSettings()
	if err != nil || state.Mode != "dark" || state.Revision != 1 {
		t.Fatal("restart lost theme", err)
	}
	if _, err = m.PluginSettings("$appearance"); err == nil {
		t.Fatal("plugin accessed host namespace")
	}
	if err = os.Rename(filepath.Join(root, ".sinan-state", "settings.json"), filepath.Join(root, ".sinan-state", "settings.previous")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, ".sinan-state", "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveTheme("light", 1); err == nil {
		t.Fatal("failed write reported success")
	}
	state, _ = m.ThemeSettings()
	if state.Mode != "dark" || state.Revision != 1 {
		t.Fatal("failed write changed theme")
	}
}
func TestMalformedThemeFallsBackWithoutOverwrite(t *testing.T) {
	m := manager(t)
	if _, err := m.settings.Update(themeNamespace, 0, map[string][]string{"mode": {"unknown"}}); err != nil {
		t.Fatal(err)
	}
	state, err := m.ThemeSettings()
	if err != nil || state.Mode != "system" || state.Error == "" {
		t.Fatal("missing fallback", err)
	}
	if _, err = m.SaveTheme("light", state.Revision); err == nil {
		t.Fatal("corrupt theme silently overwritten")
	}
	if m.settings.Get(themeNamespace).Values["mode"][0] != "unknown" {
		t.Fatal("original configuration lost")
	}
	m.Close()
	if _, err = m.ThemeSettings(); err == nil {
		t.Fatal("closed manager accepted theme access")
	}
}
