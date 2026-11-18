package kernel

import (
	"errors"
	"strconv"
)

const (
	launcherPositionNamespace = "$launcher-position"
	DefaultLauncherTopOffset  = 20
	maxLauncherTopOffset      = 50
)

type LauncherPositionPreferences struct {
	TopOffsetPercent int    `json:"topOffsetPercent"`
	Revision         uint64 `json:"revision"`
	Error            string `json:"error,omitempty"`
}

func (m *Manager) LauncherPositionPreferences() (LauncherPositionPreferences, error) {
	snapshot, err := m.hostShortcutPreferences(launcherPositionNamespace)
	state := LauncherPositionPreferences{TopOffsetPercent: DefaultLauncherTopOffset, Revision: snapshot.Revision}
	if err != nil || len(snapshot.Values) == 0 {
		return state, err
	}
	values := snapshot.Values["topOffsetPercent"]
	if len(snapshot.Values) == 1 && len(values) == 1 {
		percent, err := strconv.Atoi(values[0])
		if err == nil && validLauncherTopOffset(percent) && strconv.Itoa(percent) == values[0] {
			state.TopOffsetPercent = percent
			return state, nil
		}
	}
	state.Error = "搜索框顶边距配置无效，已使用默认值并保留原记录"
	return state, nil
}

func (m *Manager) SaveLauncherPositionPreferences(percent int, revision uint64) (LauncherPositionPreferences, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.LauncherPositionPreferences()
	if err != nil {
		return current, err
	}
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	if !validLauncherTopOffset(percent) {
		return current, errors.New("搜索框顶边距必须为 0 至 50 的整数百分比")
	}
	if current.Revision != revision {
		return current, errors.New("搜索框位置已变化，请重新读取后重试")
	}
	if current.TopOffsetPercent == percent {
		return current, nil
	}
	snapshot, err := m.settings.Update(launcherPositionNamespace, revision, map[string][]string{"topOffsetPercent": {strconv.Itoa(percent)}})
	if err != nil {
		return current, err
	}
	return LauncherPositionPreferences{TopOffsetPercent: percent, Revision: snapshot.Revision}, nil
}

func validLauncherTopOffset(percent int) bool {
	return percent >= 0 && percent <= maxLauncherTopOffset
}
