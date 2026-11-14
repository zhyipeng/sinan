package kernel

import (
	"errors"
	"sinan/internal/plugin/manifest"
	"sinan/internal/storage"
)

// Overrides are host-owned and survive plugin reload, upgrade and uninstall.
func pluginShortcutNamespace(pluginID, commandID string) (string, error) {
	if !manifest.ValidID(pluginID) || !manifest.ValidViewID(commandID) {
		return "", errors.New("插件或命令标识无效")
	}
	return "$plugin-shortcut/" + pluginID + "/" + commandID, nil
}
func (m *Manager) PluginShortcutPreferences(pluginID, commandID string) (storage.SettingsSnapshot, error) {
	namespace, err := pluginShortcutNamespace(pluginID, commandID)
	if err != nil {
		return storage.SettingsSnapshot{}, err
	}
	return m.hostShortcutPreferences(namespace)
}
func (m *Manager) SavePluginShortcutPreferences(pluginID, commandID string, revision uint64, value string) (storage.SettingsSnapshot, error) {
	namespace, err := pluginShortcutNamespace(pluginID, commandID)
	if err != nil {
		return storage.SettingsSnapshot{}, err
	}
	return m.saveHostShortcutPreferences(namespace, revision, value)
}
