package kernel

import (
	"errors"

	"sinan/internal/storage"
)

// Reserved host namespace cannot collide with a valid manifest plugin ID.
const launcherPreferencesNamespace = "$launcher"

func (m *Manager) LauncherPreferences() (storage.SettingsSnapshot, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return storage.SettingsSnapshot{}, errors.New("运行时已关闭")
	}
	if err := m.settings.Error(); err != nil {
		return storage.SettingsSnapshot{}, err
	}
	return m.settings.Get(launcherPreferencesNamespace), nil
}
func (m *Manager) SaveLauncherPreferences(revision uint64, shortcut string) (storage.SettingsSnapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return storage.SettingsSnapshot{}, errors.New("运行时已关闭")
	}
	if shortcut == "" || len(shortcut) > 128 {
		return storage.SettingsSnapshot{}, errors.New("快捷键格式无效")
	}
	return m.settings.Update(launcherPreferencesNamespace, revision, map[string][]string{"shortcut": {shortcut}})
}
