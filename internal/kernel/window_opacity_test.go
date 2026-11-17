package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowOpacityPreferencesPersistAndValidate(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	state, err := m.WindowOpacityPreferences()
	if err != nil || state.Percent != 100 || state.Revision != 0 {
		t.Fatalf("defaults: %+v %v", state, err)
	}
	for _, value := range []int{-1, 0, 29, 101} {
		if _, err = m.SaveWindowOpacityPreferences(value, 0); err == nil {
			t.Fatal("invalid opacity accepted")
		}
	}
	state, err = m.SaveWindowOpacityPreferences(30, 0)
	if err != nil || state.Percent != 30 || state.Revision != 1 {
		t.Fatalf("save: %+v %v", state, err)
	}
	if _, err = m.SaveWindowOpacityPreferences(90, 0); err == nil {
		t.Fatal("stale save accepted")
	}
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.WindowOpacityPreferences()
	if err != nil || state.Percent != 30 || state.Revision != 1 {
		t.Fatalf("restart: %+v %v", state, err)
	}
	state, err = m.SaveWindowOpacityPreferences(100, 1)
	if err != nil || state.Percent != 100 {
		t.Fatalf("restore: %+v %v", state, err)
	}
	if _, err = m.PluginSettings(windowOpacityNamespace); err == nil {
		t.Fatal("plugin accessed host preferences")
	}
	m.Close()
	if _, err = m.WindowOpacityPreferences(); err == nil {
		t.Fatal("closed runtime accepted")
	}
}

func TestWindowOpacityPreferencesInvalidDataPreserved(t *testing.T) {
	for _, values := range []map[string][]string{
		{"percent": {"0"}}, {"percent": {"101"}}, {"percent": {"NaN"}}, {"percent": {"50", "60"}}, {"percent": {"050"}}, {"unknown": {"50"}},
	} {
		m := manager(t)
		if _, err := m.settings.Update(windowOpacityNamespace, 0, values); err != nil {
			t.Fatal(err)
		}
		state, err := m.WindowOpacityPreferences()
		if err != nil || state.Percent != 100 || state.Error == "" {
			t.Fatalf("invalid data: %+v %v", state, err)
		}
		if _, err = m.SaveWindowOpacityPreferences(50, state.Revision); err == nil {
			t.Fatal("invalid record overwritten")
		}
		if m.settings.Get(windowOpacityNamespace).Revision != 1 {
			t.Fatal("invalid record changed")
		}
	}
}

func TestWindowOpacityPreferencesFailedWritePreserved(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err = m.SaveWindowOpacityPreferences(75, 0); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".sinan-state", "settings.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveWindowOpacityPreferences(50, 1); err == nil {
		t.Fatal("failed write accepted")
	}
	state, err := m.WindowOpacityPreferences()
	if err != nil || state.Percent != 75 || state.Revision != 1 {
		t.Fatalf("failed write changed state: %+v %v", state, err)
	}
}
