package kernel

import "testing"

func TestPluginShortcutPreferencesAreHostOwnedAndIndependent(t *testing.T) {
	m := manager(t)
	first, err := m.SavePluginShortcutPreferences("tools.clipboard", "open", 0, "Ctrl+Alt+V")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SavePluginShortcutPreferences("tools.other", "open", 0, "Ctrl+Alt+U"); err != nil {
		t.Fatal("unrelated revision interfered", err)
	}
	if _, err := m.SavePluginShortcutPreferences("tools.clipboard", "open", 0, "Ctrl+Alt+P"); err == nil {
		t.Fatal("stale revision accepted")
	}
	value, err := m.PluginShortcutPreferences("tools.clipboard", "open")
	if err != nil || value.Revision != first.Revision || value.Values["shortcut"][0] != "Ctrl+Alt+V" {
		t.Fatal("preferences changed")
	}
	host, _ := m.LauncherPreferences()
	if host.Revision != 0 {
		t.Fatal("plugin shortcut changed launcher revision")
	}
	root := m.root
	m.Close()
	restored, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	after, err := restored.PluginShortcutPreferences("tools.clipboard", "open")
	if err != nil || after.Revision != first.Revision || after.Values["shortcut"][0] != "Ctrl+Alt+V" {
		t.Fatal("restarted runtime lost shortcut")
	}

	for _, id := range []string{"../invalid", "$launcher"} {
		if _, err := restored.SavePluginShortcutPreferences(id, "open", 0, "Ctrl+Alt+V"); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
	if _, err := restored.PluginSettings("$plugin-shortcut/tools.clipboard/open"); err == nil {
		t.Fatal("host shortcut exposed through plugin settings")
	}
}
