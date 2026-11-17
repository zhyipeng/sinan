package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetentionPreferencesPersistValidateAndRejectStale(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	state, err := m.RetentionPreferences()
	if err != nil || state.ClearInputSeconds != 0 || state.PluginUISeconds != 0 || state.Revision != 0 {
		t.Fatalf("defaults: %+v %v", state, err)
	}
	for _, invalid := range []int{-1, 86401} {
		if _, err = m.SaveRetentionPreferences(invalid, 0, 0); err == nil {
			t.Fatal("invalid input delay")
		}
		if _, err = m.SaveRetentionPreferences(0, invalid, 0); err == nil {
			t.Fatal("invalid UI duration")
		}
	}
	state, err = m.SaveRetentionPreferences(10, 60, 0)
	if err != nil || state.Revision != 1 {
		t.Fatalf("save: %+v %v", state, err)
	}
	if _, err = m.SaveRetentionPreferences(0, 0, 0); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err = m.PluginSettings(retentionNamespace); err == nil {
		t.Fatal("plugin read host namespace")
	}
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err = m.RetentionPreferences()
	if err != nil || state.ClearInputSeconds != 10 || state.PluginUISeconds != 60 || state.Revision != 1 {
		t.Fatalf("restart: %+v %v", state, err)
	}
	path := filepath.Join(root, ".sinan-state", "settings.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveRetentionPreferences(0, 0, 1); err == nil {
		t.Fatal("failed write accepted")
	}
	state, _ = m.RetentionPreferences()
	if state.ClearInputSeconds != 10 || state.PluginUISeconds != 60 || state.Revision != 1 {
		t.Fatal("failed write changed preferences")
	}
	m.Close()
	if _, err = m.RetentionPreferences(); err == nil {
		t.Fatal("closed runtime accepted access")
	}
}

func TestRetentionPreferencesCorruptNamespacePreserved(t *testing.T) {
	for _, values := range []map[string][]string{
		{"clearInputSeconds": {"10"}},
		{"clearInputSeconds": {"-1"}, "pluginUISeconds": {"10"}},
		{"clearInputSeconds": {"1.5"}, "pluginUISeconds": {"10"}},
		{"clearInputSeconds": {"10"}, "pluginUISeconds": {"10", "20"}},
		{"clearInputSeconds": {"10"}, "pluginUISeconds": {"86401"}},
	} {
		m := manager(t)
		if _, err := m.settings.Update(retentionNamespace, 0, values); err != nil {
			t.Fatal(err)
		}
		state, err := m.RetentionPreferences()
		if err != nil || state.Error == "" || state.ClearInputSeconds != 0 || state.PluginUISeconds != 0 {
			t.Fatalf("corruption: %+v %v", state, err)
		}
		if _, err = m.SaveRetentionPreferences(0, 0, 1); err == nil {
			t.Fatal("invalid record overwritten")
		}
		if m.settings.Get(retentionNamespace).Revision != 1 {
			t.Fatal("invalid record changed")
		}
	}
}
