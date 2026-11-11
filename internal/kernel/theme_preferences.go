package kernel

import (
	"errors"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
)

const themeNamespace = "$appearance"

func (m *Manager) ThemeSettings() (protocol.ThemeState, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return protocol.ThemeState{}, errors.New("运行时已关闭")
	}
	snapshot := m.settings.Get(themeNamespace)
	state := theme.Builtins("system", snapshot.Revision)
	invalid := errors.New("主题配置无效，已跟随系统且保留原记录")
	if m.settings.Error() != nil {
		state.Error = invalid.Error()
		return state, nil
	}
	if len(snapshot.Values) == 0 {
		return state, nil
	}
	values := snapshot.Values["mode"]
	if len(snapshot.Values) != 1 || len(values) != 1 || !theme.ValidMode(values[0]) {
		state.Error = invalid.Error()
		return state, nil
	}
	state.Mode = values[0]
	return state, nil
}
func (m *Manager) SaveTheme(mode string, revision uint64) (protocol.ThemeState, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.ThemeSettings()
	if err != nil {
		return protocol.ThemeState{}, err
	}
	if current.Error != "" {
		return protocol.ThemeState{}, errors.New(current.Error)
	}
	if !theme.ValidMode(mode) {
		return protocol.ThemeState{}, errors.New("主题仅支持浅色、深色或跟随系统")
	}
	if current.Revision != revision {
		return protocol.ThemeState{}, errors.New("主题设置已变化，请重新读取后重试")
	}
	if current.Mode == mode {
		return current, nil
	}
	if _, err = m.settings.Update(themeNamespace, revision, map[string][]string{"mode": {mode}}); err != nil {
		return protocol.ThemeState{}, err
	}
	return m.ThemeSettings()
}
