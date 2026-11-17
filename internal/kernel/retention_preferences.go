package kernel

import (
	"errors"
	"strconv"
)

const retentionNamespace = "$main-retention"
const MaxRetentionSeconds = 86400

type RetentionPreferences struct {
	ClearInputSeconds int    `json:"clearInputSeconds"`
	PluginUISeconds   int    `json:"pluginUISeconds"`
	Revision          uint64 `json:"revision"`
	Error             string `json:"error,omitempty"`
}

func (m *Manager) RetentionPreferences() (RetentionPreferences, error) {
	snapshot, err := m.hostShortcutPreferences(retentionNamespace)
	state := RetentionPreferences{Revision: snapshot.Revision}
	if err != nil {
		return state, err
	}
	if len(snapshot.Values) == 0 {
		return state, nil
	}
	parse := func(key string) (int, bool) {
		values := snapshot.Values[key]
		if len(values) != 1 {
			return 0, false
		}
		value, err := strconv.Atoi(values[0])
		return value, err == nil && value >= 0 && value <= MaxRetentionSeconds && strconv.Itoa(value) == values[0]
	}
	input, okInput := parse("clearInputSeconds")
	plugin, okPlugin := parse("pluginUISeconds")
	if len(snapshot.Values) != 2 || !okInput || !okPlugin {
		state.Error = "主界面保留配置无效，已关闭保留并保留原记录"
		return state, nil
	}
	state.ClearInputSeconds = input
	state.PluginUISeconds = plugin
	return state, nil
}

func (m *Manager) SaveRetentionPreferences(clearInputSeconds, pluginUISeconds int, revision uint64) (RetentionPreferences, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.RetentionPreferences()
	if err != nil {
		return current, err
	}
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	if clearInputSeconds < 0 || clearInputSeconds > MaxRetentionSeconds || pluginUISeconds < 0 || pluginUISeconds > MaxRetentionSeconds {
		return current, errors.New("保留时长必须为 0 至 86400 的整数秒")
	}
	if current.Revision != revision {
		return current, errors.New("主界面保留设置已变化，请重新读取后重试")
	}
	if current.ClearInputSeconds == clearInputSeconds && current.PluginUISeconds == pluginUISeconds {
		return current, nil
	}
	_, err = m.settings.Update(retentionNamespace, revision, map[string][]string{"clearInputSeconds": {strconv.Itoa(clearInputSeconds)}, "pluginUISeconds": {strconv.Itoa(pluginUISeconds)}})
	if err != nil {
		return current, err
	}
	return m.RetentionPreferences()
}
