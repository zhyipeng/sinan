package kernel

import (
	"errors"
	"os"
	"path/filepath"
	"sinan/internal/plugin/manifest"
	"sinan/internal/storage"
	"strings"
)

func settingsValues(m manifest.Manifest, saved map[string][]string) map[string][]string {
	values := map[string][]string{}
	for _, field := range m.Contributes.Settings {
		values[field.ID] = append([]string{}, saved[field.ID]...)
	}
	return values
}
func (m *Manager) PluginSettings(pluginID string) (storage.SettingsSnapshot, error) {
	p, err := m.lookup(pluginID)
	if err != nil {
		return storage.SettingsSnapshot{}, err
	}
	if err = m.settings.Error(); err != nil {
		return storage.SettingsSnapshot{}, err
	}
	value := m.settings.Get(pluginID)
	value.Values = settingsValues(p.manifest, value.Values)
	return value, nil
}
func (m *Manager) SavePluginSettings(pluginID string, revision uint64, values map[string][]string) (storage.SettingsSnapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(pluginID)
	if err != nil {
		return storage.SettingsSnapshot{}, err
	}
	allowed := map[string]manifest.Setting{}
	for _, field := range p.manifest.Contributes.Settings {
		allowed[field.ID] = field
	}
	normalized := map[string][]string{}
	for key, values := range values {
		field, ok := allowed[key]
		if !ok {
			return storage.SettingsSnapshot{}, errors.New("设置项未声明")
		}
		switch field.Type {
		case "select":
			if len(values) > 1 {
				return storage.SettingsSnapshot{}, errors.New("设置项取值无效")
			}
			value := ""
			if len(values) == 1 {
				value = strings.TrimSpace(values[0])
			}
			if value == "" {
				normalized[key] = []string{}
				continue
			}
			valid := false
			for _, option := range field.Options {
				if option.Value == value {
					valid = true
					break
				}
			}
			if !valid {
				return storage.SettingsSnapshot{}, errors.New("设置项取值无效")
			}
			normalized[key] = []string{value}
		case "text":
			if len(values) > 1 {
				return storage.SettingsSnapshot{}, errors.New("设置项取值无效")
			}
			value := ""
			if len(values) == 1 {
				value = strings.TrimSpace(values[0])
			}
			if len(value) > 2048 || strings.ContainsRune(value, 0) {
				return storage.SettingsSnapshot{}, errors.New("设置内容过长或无效")
			}
			if value == "" {
				normalized[key] = []string{}
			} else {
				normalized[key] = []string{value}
			}
		default: // directory-list
			if len(values) > 16 {
				return storage.SettingsSnapshot{}, errors.New("设置项未声明或目录数量超过 16 个")
			}
			seen := map[string]bool{}
			normalized[key] = []string{}
			for _, path := range values {
				path = strings.TrimSpace(path)
				if len(path) > 1024 || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
					return storage.SettingsSnapshot{}, errors.New("请输入有效的目录绝对路径")
				}
				path = filepath.Clean(path)
				info, statErr := os.Stat(path)
				if statErr != nil || !info.IsDir() {
					return storage.SettingsSnapshot{}, errors.New("目录不存在或无法访问，请检查后重试")
				}
				if !seen[path] {
					seen[path] = true
					normalized[key] = append(normalized[key], path)
				}
			}
		}
	}
	normalized = settingsValues(p.manifest, normalized)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removed {
		return storage.SettingsSnapshot{}, errors.New("插件已移除")
	}
	saved, err := m.settings.Update(pluginID, revision, normalized)
	if err != nil {
		return storage.SettingsSnapshot{}, err
	}
	// Serialize snapshot replacement with activation. The next request initializes
	// a new process with only this plugin's configuration namespace.
	p.settings = settingsValues(p.manifest, saved.Values)
	p.revokeViewsLocked()
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	p.state = "deactivated"
	return saved, nil
}
