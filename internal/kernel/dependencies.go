package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"sinan/internal/paste"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

type DependencyTarget struct{ PluginID, CommandID, Fingerprint string }

func (m *Manager) dependencyState(d protocol.Dependency) (protocol.DependencyStatus, *plugin) {
	s := protocol.DependencyStatus{ID: d.ID, PluginID: d.PluginID, Optional: d.Optional, MinVersion: d.MinVersion, State: "available"}
	fail := func(state, message string) (protocol.DependencyStatus, *plugin) {
		s.State = state
		s.Message = message
		return s, nil
	}
	p, err := m.lookup(d.PluginID)
	if err != nil {
		return fail("missing", "未安装依赖插件")
	}
	p.mu.Lock()
	disabled, changed, removed := p.disabled, p.changed, p.removed
	p.mu.Unlock()
	if removed {
		return fail("missing", "依赖插件已卸载")
	}
	if disabled {
		return fail("disabled", "依赖插件已禁用")
	}
	if changed {
		return fail("changed", "依赖插件需要重载")
	}
	if manifest.CompareVersion(p.manifest.Version, d.MinVersion) < 0 {
		return fail("incompatible", "依赖插件版本过低，需要 "+d.MinVersion)
	}
	for _, need := range d.Entries {
		found := false
		for _, e := range p.manifest.Contributes.EntryPoints {
			if e.ID == need.ID && e.Version == need.Version {
				found = true
			}
		}
		if !found {
			return fail("incompatible", "依赖插件未公开所需入口或入口版本不兼容")
		}
	}
	if actual, e := packageFingerprint(p.dir); e != nil || actual != p.fingerprint {
		return fail("changed", "依赖插件包已变化，请重载")
	}
	if p.manifest.Runtime.Kind == "process" {
		info, e := os.Stat(filepath.Join(p.dir, p.manifest.Runtime.Entry))
		if e != nil || !info.Mode().IsRegular() {
			return fail("unsupported", "依赖插件原生入口不可用")
		}
	}
	return s, p
}
func (m *Manager) dependencyStatuses(p *plugin, ids []string) []protocol.DependencyStatus {
	result := []protocol.DependencyStatus{}
	for _, d := range p.manifest.Dependencies {
		if ids != nil && !slices.Contains(ids, d.ID) {
			continue
		}
		s, _ := m.dependencyState(d)
		result = append(result, s)
	}
	return result
}
func (m *Manager) checkRequired(p *plugin, visiting map[string]bool) error {
	if active, seen := visiting[p.manifest.ID]; seen {
		if active {
			return errors.New("插件硬依赖存在循环")
		}
		return nil
	}
	if len(visiting) >= 128 {
		return errors.New("硬依赖可达插件超过 128 个")
	}

	visiting[p.manifest.ID] = true
	defer func() { visiting[p.manifest.ID] = false }()
	for _, d := range p.manifest.Dependencies {
		if d.Optional {
			continue
		}
		s, target := m.dependencyState(d)
		if s.State != "available" {
			return fmt.Errorf("依赖 %s 不可用：%s", d.PluginID, s.Message)
		}
		if err := m.checkRequired(target, visiting); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) validateDependencyGraph(candidate manifest.Manifest) error {
	all := map[string]manifest.Manifest{}
	m.mu.Lock()
	for id, p := range m.plugins {
		all[id] = p.manifest
	}
	m.mu.Unlock()
	all[candidate.ID] = candidate
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return errors.New("插件硬依赖存在循环")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, d := range all[id].Dependencies {
			if !d.Optional {
				if err := visit(d.PluginID); err != nil {
					return err
				}
			}
		}
		delete(visiting, id)
		done[id] = true
		return nil
	}
	for id := range all {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// resolveDependencyLocked requires mutationMu: package identity cannot change while resolving.
func (m *Manager) resolveDependencyLocked(session *uiview.Session, alias, entry string) (DependencyTarget, error) {
	p, err := m.lookup(session.Binding().PluginID)
	if err != nil {
		return DependencyTarget{}, err
	}
	p.mu.Lock()
	_, owned := p.views[session]
	live := owned && !p.removed && !p.changed && !p.disabled && p.fingerprint == session.Binding().Fingerprint && session.Active()
	p.mu.Unlock()
	if !live {
		return DependencyTarget{}, errors.New("调用方会话不可用")
	}
	if actual, err := packageFingerprint(p.dir); err != nil || actual != p.fingerprint {
		return DependencyTarget{}, errors.New("调用方插件包已变化，请重载")
	}
	if err := m.checkRequired(p, map[string]bool{}); err != nil {
		return DependencyTarget{}, err
	}
	allowed := false
	for _, v := range p.manifest.Contributes.UIViews {
		if v.ID == session.Binding().ViewID && slices.Contains(v.Dependencies, alias) {
			allowed = true
		}
	}
	if !allowed {
		return DependencyTarget{}, errors.New("此视图未声明依赖授权")
	}
	for _, d := range p.manifest.Dependencies {
		if d.ID != alias {
			continue
		}
		requested := false
		for _, e := range d.Entries {
			if e.ID == entry {
				requested = true
			}
		}
		if !requested {
			return DependencyTarget{}, errors.New("未声明所请求的依赖入口")
		}
		status, target := m.dependencyState(d)
		if target == nil {
			return DependencyTarget{}, errors.New(status.Message)
		}
		if err := m.checkRequired(target, map[string]bool{}); err != nil {
			return DependencyTarget{}, err
		}
		actual, e := packageFingerprint(target.dir)
		if e != nil || actual != target.fingerprint {
			return DependencyTarget{}, errors.New("目标插件包已变化，请重载")
		}
		for _, export := range target.manifest.Contributes.EntryPoints {
			if export.ID == entry {
				return DependencyTarget{target.manifest.ID, export.CommandID, target.fingerprint}, nil
			}
		}
	}
	return DependencyTarget{}, errors.New("依赖入口不存在")
}
func (m *Manager) ResolveDependency(session *uiview.Session, alias, entry string) (DependencyTarget, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return m.resolveDependencyLocked(session, alias, entry)
}
func (m *Manager) callDependency(ctx context.Context, p *plugin, s *uiview.Session, view protocol.UIView, req protocol.UIRequest) (json.RawMessage, error) {
	if len(view.Dependencies) == 0 {
		return nil, errors.New("此视图未声明依赖授权")
	}
	if req.Method == "dependency.status" {
		return marshalUIResult(m.dependencyStatuses(p, view.Dependencies))
	}
	if m.uiHostCall == nil {
		return nil, errors.New("当前宿主不支持页面交接")
	}
	return m.uiHostCall(ctx, s, req)
}

// Mutation hooks revoke consumers with newly unsatisfied hard dependencies.
func (m *Manager) invalidateDependents() {
	m.mu.Lock()
	all := make([]*plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		all = append(all, p)
	}
	m.mu.Unlock()
	for _, p := range all {
		if len(p.manifest.Dependencies) > 0 && m.checkRequired(p, map[string]bool{}) != nil {
			p.mu.Lock()
			p.revokeViewsLocked()
			if p.process != nil {
				p.process.Stop()
				p.process = nil
			}
			p.mu.Unlock()
		}
	}
}

func (m *Manager) PrepareDependencyImage(ctx context.Context, source *uiview.Session, alias, entry, fingerprint string, content *paste.Content) (*uiview.Session, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	target, err := m.resolveDependencyLocked(source, alias, entry)
	if err != nil {
		return nil, err
	}
	if target.Fingerprint != fingerprint {
		return nil, errors.New("目标插件已更换，请重新交接")
	}
	p, err := m.lookup(target.PluginID)
	if err != nil {
		return nil, err
	}
	if p.manifest.Runtime.Kind == "process" {
		if _, err = p.ensureUIProcess(ctx, m.root); err != nil {
			return nil, err
		}
	}
	for _, cmd := range p.manifest.Contributes.Commands {
		if cmd.ID == target.CommandID && content != nil && content.IsImage() && content.Matches(cmd.InputMatchers) {
			s, err := m.openUIViewLocked(ctx, target.PluginID, cmd.UIView, protocol.UILaunchContext{CommandID: cmd.ID}, content)
			if err == nil {
				s.Suspend()
			}
			return s, err
		}
	}
	return nil, errors.New("目标入口不接受图片")
}
func (m *Manager) CommitDependency(ctx context.Context, source, target *uiview.Session, alias, entry string, swap func() error) error {
	m.mutationMu.Lock()
	resolved, err := m.resolveDependencyLocked(source, alias, entry)
	if err == nil && (!target.Active() || target.Binding().PluginID != resolved.PluginID || target.Binding().Fingerprint != resolved.Fingerprint) {
		err = errors.New("目标页面已失效")
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = swap()
	}
	m.mutationMu.Unlock()
	if err == nil {
		target.Activate()
	}
	return err
}
