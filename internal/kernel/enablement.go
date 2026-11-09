package kernel

import (
	"errors"
	"sort"

	"sinan/internal/plugin/manifest"
)

// Host policy cannot be addressed through a valid plugin ID or settings schema.
const enablementNamespace = "$plugins"

func (m *Manager) loadEnablement() {
	m.disabled = map[string]bool{}
	invalid := errors.New("插件启用状态无法读取，插件保持禁用，请修复配置后重启")
	if m.settings.Error() != nil {
		m.enablementErr = invalid
		return
	}
	values := m.settings.Get(enablementNamespace).Values
	for key, ids := range values {
		if key != "disabled" || len(ids) > 256 {
			m.enablementErr = invalid
			return
		}
		for _, id := range ids {
			if !manifest.ValidID(id) || m.disabled[id] {
				m.enablementErr = invalid
				return
			}
			m.disabled[id] = true
		}
	}
	// Ignore valid legacy disable entries for the now-protected system views.
	// Preserve the source file; malformed policy still follows fail-closed handling.
	for id := range m.disabled {
		if protectedPlugin(id) {
			delete(m.disabled, id)
		}
	}
}

// protectedPlugin is host policy, never a package-controlled manifest flag.
func protectedPlugin(id string) bool {
	return id == "system.settings" || id == "system.plugin-manager"
}

// SetPluginEnabled persists the desired state before changing the live instance.
// Failure to save must leave the current process and policy unchanged.
func (m *Manager) SetPluginEnabled(id string, enabled bool) (PluginInfo, error) {
	if !enabled && protectedPlugin(id) {
		return PluginInfo{}, errors.New("系统插件不允许禁用")
	}
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(id)
	if err != nil {
		return PluginInfo{}, err
	}
	if m.enablementErr != nil {
		return PluginInfo{}, m.enablementErr
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removed {
		return PluginInfo{}, errors.New("插件已移除")
	}
	if p.disabled == !enabled {
		return p.info(), nil
	}
	disabled := make([]string, 0, len(m.disabled)+1)
	for key, value := range m.disabled {
		if value && key != id {
			disabled = append(disabled, key)
		}
	}
	if !enabled {
		disabled = append(disabled, id)
	}
	if len(disabled) > 256 {
		return PluginInfo{}, errors.New("禁用插件数量超过限制")
	}
	sort.Strings(disabled)
	previous := m.settings.Get(enablementNamespace)
	if _, err = m.settings.Update(enablementNamespace, previous.Revision, map[string][]string{"disabled": disabled}); err != nil {
		return PluginInfo{}, err
	}
	if enabled {
		delete(m.disabled, id)
	} else {
		m.disabled[id] = true
	}
	p.disabled = !enabled
	p.revokeViewsLocked()
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	p.state = "installed"
	return p.info(), nil
}
