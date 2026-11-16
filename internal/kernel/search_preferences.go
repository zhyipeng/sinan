package kernel

import "errors"

const searchPreferencesNamespace = "$search"

type SearchPreferences struct {
	SpaceToOpen bool   `json:"spaceToOpen"`
	Revision    uint64 `json:"revision"`
	Error       string `json:"error,omitempty"`
}

func (m *Manager) SearchPreferences() (SearchPreferences, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return SearchPreferences{}, errors.New("运行时已关闭")
	}
	snapshot := m.settings.Get(searchPreferencesNamespace)
	state := SearchPreferences{Revision: snapshot.Revision}
	invalid := "搜索配置无效，已关闭空格键打开且保留原记录"
	if m.settings.Error() != nil {
		state.Error = invalid
		return state, nil
	}
	if len(snapshot.Values) == 0 {
		return state, nil
	}
	values := snapshot.Values["spaceToOpen"]
	if len(snapshot.Values) != 1 || len(values) != 1 || (values[0] != "true" && values[0] != "false") {
		state.Error = invalid
		return state, nil
	}
	state.SpaceToOpen = values[0] == "true"
	return state, nil
}

func (m *Manager) SaveSearchPreferences(spaceToOpen bool, revision uint64) (SearchPreferences, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	current, err := m.SearchPreferences()
	if err != nil {
		return SearchPreferences{}, err
	}
	if current.Error != "" {
		return SearchPreferences{}, errors.New(current.Error)
	}
	if current.Revision != revision {
		return SearchPreferences{}, errors.New("搜索设置已变化，请重新读取后重试")
	}
	if current.SpaceToOpen == spaceToOpen {
		return current, nil
	}
	value := "false"
	if spaceToOpen {
		value = "true"
	}
	if _, err = m.settings.Update(searchPreferencesNamespace, revision, map[string][]string{"spaceToOpen": {value}}); err != nil {
		return SearchPreferences{}, err
	}
	return m.SearchPreferences()
}
