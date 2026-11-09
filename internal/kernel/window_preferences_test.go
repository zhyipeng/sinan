package kernel

import "testing"

func TestWindowPreferencesKeepLauncherRevisionAndPluginNamespaceSeparate(t *testing.T) {
	m := manager(t)
	launcher, err := m.SaveLauncherPreferences(0, "Cmd+Space")
	if err != nil {
		t.Fatal(err)
	}
	window, err := m.SaveWindowPreferences(0, "Ctrl+Alt+U")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := m.LauncherPreferences()
	if err != nil || loaded.Revision != launcher.Revision || loaded.Values["shortcut"][0] != "Cmd+Space" {
		t.Fatal("window preference changed launcher")
	}
	if _, err := m.SaveWindowPreferences(0, "Ctrl+Alt+K"); err == nil {
		t.Fatal("stale revision overwrote window preference")
	}
	current, err := m.WindowPreferences()
	if err != nil || current.Revision != window.Revision || current.Values["shortcut"][0] != "Ctrl+Alt+U" {
		t.Fatal("failed write changed preferences")
	}
	if _, err := m.PluginSettings("$window"); err == nil {
		t.Fatal("plugin addressed reserved window preferences")
	}
}
