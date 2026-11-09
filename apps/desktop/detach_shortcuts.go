package main

import (
	"runtime"
	"sinan/internal/kernel"
	"sinan/internal/storage"
)

func defaultDetachShortcut() string {
	if runtime.GOOS == "darwin" {
		return "Shift+Cmd+D"
	}
	return "Ctrl+Shift+D"
}

// Local bindings are matched by the focused view, never reserved with the OS.
type localShortcutRegistry struct{}

func (localShortcutRegistry) Register(string, func()) error { return nil }
func (localShortcutRegistry) Unregister(string) error       { return nil }

type windowPreferences struct{ manager *kernel.Manager }

func (p windowPreferences) LauncherPreferences() (storage.SettingsSnapshot, error) {
	return p.manager.WindowPreferences()
}
func (p windowPreferences) SaveLauncherPreferences(revision uint64, shortcut string) (storage.SettingsSnapshot, error) {
	return p.manager.SaveWindowPreferences(revision, shortcut)
}
