package kernel

import (
	"encoding/hex"
	"errors"
	"slices"
	"sort"
	"strings"

	"sinan/internal/plugin/manifest"
)

const uiPermissionsNamespace = "$ui-permissions"

type uiGrant struct {
	fingerprint  string
	capabilities []string
}
type UIPermission struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Allowed bool   `json:"allowed"`
}
type UIPermissionInfo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Fingerprint string         `json:"fingerprint"`
	Reviewed    bool           `json:"reviewed"`
	Stale       bool           `json:"stale"`
	Changed     bool           `json:"changed"`
	Permissions []UIPermission `json:"permissions"`
}
type UIPermissionState struct {
	Revision uint64             `json:"revision"`
	Plugins  []UIPermissionInfo `json:"plugins"`
	Error    string             `json:"error"`
}

func validUICapability(id string) bool {
	parts := strings.Split(id, "/")
	if len(parts) < 2 || !manifest.ValidViewID(parts[0]) {
		return false
	}
	if len(parts) == 2 {
		return parts[1] == "storage"
	}
	if len(parts) != 3 || parts[1] != "command" || parts[2] == "" {
		return false
	}
	for i, c := range parts[2] {
		if !(c >= 'a' && c <= 'z' || i > 0 && (c >= '0' && c <= '9' || c == '-')) {
			return false
		}
	}
	return true
}
func (m *Manager) loadUIPermissions() {
	m.uiGrants = map[string]uiGrant{}
	invalid := errors.New("UI 权限记录无效，已拒绝能力调用，请修复配置后重启")
	if m.settings.Error() != nil {
		m.uiPermissionErr = invalid
		return
	}
	values := m.settings.Get(uiPermissionsNamespace).Values
	if len(values) > 256 {
		m.uiPermissionErr = invalid
		return
	}
	for id, record := range values {
		if !manifest.ValidID(id) || len(record) < 1 || len(record) > 69 {
			m.uiPermissionErr = invalid
			return
		}
		digest, err := hex.DecodeString(record[0])
		if err != nil || len(digest) != 32 {
			m.uiPermissionErr = invalid
			return
		}
		seen := map[string]bool{}
		for _, cap := range record[1:] {
			if !validUICapability(cap) || seen[cap] {
				m.uiPermissionErr = invalid
				return
			}
			seen[cap] = true
		}
		m.uiGrants[id] = uiGrant{record[0], append([]string(nil), record[1:]...)}
	}
}
func declaredUIPermissions(mf manifest.Manifest) []UIPermission {
	result := []UIPermission{}
	for _, view := range mf.Contributes.UIViews {
		if view.BridgeVersion != "2" {
			continue
		}
		title := view.Title
		if title == "" && len(mf.Contributes.UIViews) > 1 {
			title = view.ID
		}
		suffix := ""
		if title != "" {
			suffix = "（" + title + "）"
		}
		for _, id := range view.Commands {
			name := id
			for _, command := range mf.Contributes.Commands {
				if command.ID == id {
					name = command.Title
					break
				}
			}
			result = append(result, UIPermission{ID: view.ID + "/command/" + id, Title: "运行命令：" + name + suffix})
		}
		if slices.Contains(view.Permissions, "storage") {
			result = append(result, UIPermission{ID: view.ID + "/storage", Title: "读写私有数据" + suffix})
		}
	}
	return result
}
func (m *Manager) uiAllowed(p *plugin, capability string) bool {
	grant := m.uiGrants[p.manifest.ID]
	return m.uiPermissionErr == nil && grant.fingerprint == p.fingerprint && slices.Contains(grant.capabilities, capability)
}
func (m *Manager) uiPermissionStateLocked() UIPermissionState {
	state := UIPermissionState{Revision: m.settings.Get(uiPermissionsNamespace).Revision, Plugins: []UIPermissionInfo{}}
	if m.uiPermissionErr != nil {
		state.Error = m.uiPermissionErr.Error()
	}
	m.mu.Lock()
	entries := make([]*plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		entries = append(entries, p)
	}
	m.mu.Unlock()
	for _, p := range entries {
		permissions := declaredUIPermissions(p.manifest)
		if len(permissions) == 0 {
			continue
		}
		p.mu.Lock()
		changed := p.changed
		p.mu.Unlock()
		actual, err := packageFingerprint(p.dir)
		if err != nil || actual != p.fingerprint {
			p.markUIChanged()
			changed = true
		}
		grant, exists := m.uiGrants[p.manifest.ID]
		for i := range permissions {
			permissions[i].Allowed = !changed && m.uiAllowed(p, permissions[i].ID)
		}
		state.Plugins = append(state.Plugins, UIPermissionInfo{ID: p.manifest.ID, Name: p.manifest.Name, Fingerprint: p.fingerprint, Reviewed: exists && grant.fingerprint == p.fingerprint && !changed, Stale: exists && (grant.fingerprint != p.fingerprint || changed), Changed: changed, Permissions: permissions})
	}
	sort.Slice(state.Plugins, func(i, j int) bool { return state.Plugins[i].ID < state.Plugins[j].ID })
	return state
}
func (m *Manager) UIPermissions() (UIPermissionState, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return UIPermissionState{}, errors.New("运行时已关闭")
	}
	return m.uiPermissionStateLocked(), nil
}
func (m *Manager) SaveUIPermissions(id, fingerprint string, revision uint64, capabilities []string) (UIPermissionState, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.uiPermissionErr != nil {
		return UIPermissionState{}, m.uiPermissionErr
	}
	p, err := m.lookup(id)
	if err != nil {
		return UIPermissionState{}, err
	}
	if fingerprint != p.fingerprint {
		return UIPermissionState{}, errors.New("插件包已变化，请刷新权限后重新确认")
	}
	p.mu.Lock()
	changed := p.changed
	p.mu.Unlock()
	actual, err := packageFingerprint(p.dir)
	if err != nil || actual != p.fingerprint || changed {
		p.markUIChanged()
		return UIPermissionState{}, errors.New("插件文件已变化，请先重载")
	}
	declared := declaredUIPermissions(p.manifest)
	allowed := map[string]bool{}
	for _, cap := range declared {
		allowed[cap.ID] = true
	}
	if len(declared) == 0 {
		return UIPermissionState{}, errors.New("此插件没有可授权的 UI 能力")
	}
	seen := map[string]bool{}
	for _, cap := range capabilities {
		if !allowed[cap] || seen[cap] {
			return UIPermissionState{}, errors.New("权限未声明或重复")
		}
		seen[cap] = true
	}
	current := m.settings.Get(uiPermissionsNamespace)
	if current.Revision != revision {
		return UIPermissionState{}, errors.New("权限设置已变化，请刷新后重试")
	}
	selected := append([]string{}, capabilities...)
	sort.Strings(selected)
	previous := m.uiGrants[id]
	if previous.fingerprint == fingerprint && slices.Equal(previous.capabilities, selected) {
		return m.uiPermissionStateLocked(), nil
	}
	if current.Values == nil {
		current.Values = map[string][]string{}
	}
	if _, exists := current.Values[id]; !exists && len(current.Values) >= 256 {
		return UIPermissionState{}, errors.New("权限记录数量已达上限")
	}
	current.Values[id] = append([]string{fingerprint}, selected...)
	if _, err = m.settings.Update(uiPermissionsNamespace, revision, current.Values); err != nil {
		return UIPermissionState{}, err
	}
	m.uiGrants[id] = uiGrant{fingerprint, selected}
	p.mu.Lock()
	p.revokeViewsLocked()
	p.mu.Unlock()
	return m.uiPermissionStateLocked(), nil
}
