package kernel

import (
	"errors"
	"strconv"
)

const windowOpacityNamespace = "$window-opacity"

type WindowOpacityPreferences struct {
	Percent  int    `json:"percent"`
	Revision uint64 `json:"revision"`
	Error    string `json:"error,omitempty"`
}

func (m *Manager) WindowOpacityPreferences() (WindowOpacityPreferences, error) {
	snapshot, err := m.hostShortcutPreferences(windowOpacityNamespace)
	state := WindowOpacityPreferences{Percent: 100, Revision: snapshot.Revision}
	if err != nil {
		return state, err
	}
	if len(snapshot.Values) == 0 {
		return state, nil
	}
	values := snapshot.Values["percent"]
	if len(snapshot.Values) == 1 && len(values) == 1 {
		value, err := strconv.Atoi(values[0])
		if err == nil && value >= 30 && value <= 100 && strconv.Itoa(value) == values[0] {
			state.Percent = value
			return state, nil
		}
	}
	state.Error = "窗口透明度配置无效，已恢复不透明并保留原记录"
	return state, nil
}

func (m *Manager) SaveWindowOpacityPreferences(percent int, revision uint64) (WindowOpacityPreferences, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.WindowOpacityPreferences()
	if err != nil {
		return current, err
	}
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	if percent < 30 || percent > 100 {
		return current, errors.New("窗口不透明度必须为 30 至 100 的整数")
	}
	if current.Revision != revision {
		return current, errors.New("窗口透明度已变化，请重新读取后重试")
	}
	if current.Percent == percent {
		return current, nil
	}
	if _, err := m.settings.Update(windowOpacityNamespace, revision, map[string][]string{"percent": {strconv.Itoa(percent)}}); err != nil {
		return current, err
	}
	return m.WindowOpacityPreferences()
}
