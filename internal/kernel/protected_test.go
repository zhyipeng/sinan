package kernel

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func installProtectedViews(t *testing.T, m *Manager) {
	t.Helper()
	for _, dir := range []string{"system-settings", "system-plugin-manager"} {
		info, err := m.Install(filepath.Join("..", "..", "plugins", dir), false)
		if err != nil || !info.Protected || info.Disabled {
			t.Fatal("protected installation failed", err)
		}
	}
}

func TestSystemViewsRejectDisableAndUninstallWithoutMutation(t *testing.T) {
	m := manager(t)
	installProtectedViews(t, m)
	before := m.settings.Get(enablementNamespace)
	for _, id := range []string{"system.settings", "system.plugin-manager"} {
		if _, err := m.SetPluginEnabled(id, false); err == nil {
			t.Fatal("system view disabled", id)
		}
		if err := m.Uninstall(id); err == nil {
			t.Fatal("system view uninstalled", id)
		}
		if _, err := os.Stat(filepath.Join(m.root, id, "manifest.json")); err != nil {
			t.Fatal("rejection removed package", err)
		}
		info, err := m.SetPluginEnabled(id, true)
		if err != nil || !info.Protected || info.Disabled {
			t.Fatal("protected view lost enabled state", err)
		}
	}
	if !reflect.DeepEqual(before, m.settings.Get(enablementNamespace)) {
		t.Fatal("rejected operations mutated policy")
	}
	ordinary, err := m.Install(source(t, false), false)
	if err != nil || ordinary.Protected {
		t.Fatal("ordinary plugin incorrectly protected", err)
	}
	if _, err := m.SetPluginEnabled(ordinary.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(ordinary.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyDisableEntriesDoNotDisableProtectedViews(t *testing.T) {
	m := manager(t)
	installProtectedViews(t, m)
	if _, err := m.Install(source(t, false), false); err != nil {
		t.Fatal(err)
	}
	before := m.settings.Get(enablementNamespace)
	if _, err := m.settings.Update(enablementNamespace, before.Revision, map[string][]string{
		"disabled": {"system.settings", "system.plugin-manager", "example.hello"},
	}); err != nil {
		t.Fatal(err)
	}
	root := m.root
	m.Close()
	path := filepath.Join(root, ".sinan-state", "settings.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	for _, info := range next.Snapshot().Plugins {
		if info.Protected && info.Disabled {
			t.Fatal("legacy policy disabled protected plugin")
		}
		if info.ID == "example.hello" && !info.Disabled {
			t.Fatal("ordinary plugin disable state lost")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("loading policy rewrote source file")
	}
}
