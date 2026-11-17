package kernel

import (
	"os"
	"path/filepath"
	"sinan/internal/storage"
	"testing"
)

func TestStartupPreferencesPersistAndRejectStaleSaves(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	state, err := m.StartupPreferences()
	if err != nil || state.Silent || state.Revision != 0 || state.Error != "" {
		t.Fatalf("unexpected defaults: %+v, %v", state, err)
	}
	state, err = m.SaveStartupPreferences(true, 0)
	if err != nil || !state.Silent || state.Revision != 1 {
		t.Fatalf("save failed: %+v, %v", state, err)
	}
	if _, err = m.SaveStartupPreferences(false, 0); err == nil {
		t.Fatal("stale save accepted")
	}
	if _, err = m.PluginSettings(startupPreferencesNamespace); err == nil {
		t.Fatal("plugin accessed host preferences")
	}
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.StartupPreferences()
	if err != nil || !state.Silent || state.Revision != 1 {
		t.Fatalf("restart lost preferences: %+v, %v", state, err)
	}
	state, err = m.SaveStartupPreferences(false, 1)
	if err != nil || state.Silent || state.Revision != 2 {
		t.Fatalf("disable failed: %+v, %v", state, err)
	}
	// A failed disk replacement must preserve the last saved value and revision.
	path := filepath.Join(root, ".sinan-state", "settings.json")
	if err = os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveStartupPreferences(true, 2); err == nil {
		t.Fatal("failed write reported success")
	}
	state, _ = m.StartupPreferences()
	if state.Silent || state.Revision != 2 {
		t.Fatalf("failed write changed preferences: %+v", state)
	}
}

func TestStartupPreferencesInvalidDataStaysIntact(t *testing.T) {
	for _, values := range []map[string][]string{
		{"silent": {"invalid"}},
		{"silent": {"true", "false"}},
		{"unknown": {"true"}},
	} {
		m := manager(t)
		if _, err := m.settings.Update(startupPreferencesNamespace, 0, values); err != nil {
			t.Fatal(err)
		}
		state, err := m.StartupPreferences()
		if err != nil || state.Silent || state.Error == "" {
			t.Fatalf("invalid configuration not rejected: %+v, %v", state, err)
		}
		if _, err = m.SaveStartupPreferences(true, state.Revision); err == nil {
			t.Fatal("invalid configuration overwritten")
		}
		if m.settings.Get(startupPreferencesNamespace).Revision != 1 {
			t.Fatal("invalid record changed")
		}
		m.Close()
		if _, err = m.StartupPreferences(); err == nil {
			t.Fatal("closed manager accepted access")
		}
	}
}

func TestStartupPreferencesCorruptFileRemainsDisabled(t *testing.T) {
	m := manager(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	m.settings = storage.OpenSettings(path)
	state, err := m.StartupPreferences()
	if err != nil || state.Silent || state.Error == "" {
		t.Fatalf("corrupt file not rejected: %+v, %v", state, err)
	}
	if _, err = m.SaveStartupPreferences(true, state.Revision); err == nil {
		t.Fatal("corrupt file overwritten")
	}
}
