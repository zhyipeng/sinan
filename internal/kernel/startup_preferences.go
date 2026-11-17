package kernel

import "errors"

const startupPreferencesNamespace = "$startup"

type StartupPreferences struct {
	Silent   bool   `json:"silent"`
	Revision uint64 `json:"revision"`
	Error    string `json:"error,omitempty"`
}

func (m *Manager) StartupPreferences() (StartupPreferences, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return StartupPreferences{}, errors.New("运行时已关闭")
	}
	snapshot := m.settings.Get(startupPreferencesNamespace)
	state := StartupPreferences{Revision: snapshot.Revision}
	invalid := "启动配置无效，已关闭静默启动且保留原记录"
	if m.settings.Error() != nil {
		state.Error = invalid
		return state, nil
	}
	if len(snapshot.Values) == 0 {
		return state, nil
	}
	values := snapshot.Values["silent"]
	if len(snapshot.Values) != 1 || len(values) != 1 || (values[0] != "true" && values[0] != "false") {
		state.Error = invalid
		return state, nil
	}
	state.Silent = values[0] == "true"
	return state, nil
}

func (m *Manager) SaveStartupPreferences(silent bool, revision uint64) (StartupPreferences, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.StartupPreferences()
	if err != nil {
		return StartupPreferences{}, err
	}
	if current.Error != "" {
		return StartupPreferences{}, errors.New(current.Error)
	}
	if current.Revision != revision {
		return StartupPreferences{}, errors.New("启动设置已变化，请重新读取后重试")
	}
	if current.Silent == silent {
		return current, nil
	}
	value := "false"
	if silent {
		value = "true"
	}
	if _, err = m.settings.Update(startupPreferencesNamespace, revision, map[string][]string{"silent": {value}}); err != nil {
		return StartupPreferences{}, err
	}
	return m.StartupPreferences()
}
