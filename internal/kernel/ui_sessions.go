package kernel

import (
	"context"
	"errors"
	"path/filepath"

	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

const maxUIPluginSessions = 4
const maxUISessions = 16

// OpenUIView prepares a declared view by ID; it is not exposed through Wails.
// Only the installed package's fixed ui/ subtree can supply resources.
func (m *Manager) OpenUIView(ctx context.Context, id, viewID string) (*uiview.Session, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return m.openUIViewLocked(ctx, id, viewID)
}

// OpenUICommand resolves the current installed command and opens its view under
// one mutation lock, so reload/upgrade cannot replace the binding in between.
func (m *Manager) OpenUICommand(ctx context.Context, id, commandID string) (*uiview.Session, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := m.lookup(id)
	if err != nil {
		return nil, err
	}
	for _, command := range p.manifest.Contributes.Commands {
		if command.ID == commandID && command.UIView != "" {
			return m.openUIViewLocked(ctx, id, command.UIView)
		}
	}
	return nil, errors.New("命令不存在或未声明独立视图")
}

func (m *Manager) openUIViewLocked(ctx context.Context, id, viewID string) (*uiview.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := m.lookup(id)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.removed || p.disabled || p.changed {
		p.mu.Unlock()
		return nil, errors.New("插件不可用，请启用或重载后重试")
	}
	var declaration *protocol.UIView
	for _, view := range p.manifest.Contributes.UIViews {
		if view.ID == viewID {
			declaration = &view
			break
		}
	}
	if declaration == nil || declaration.BridgeVersion != protocol.UIBridgeVersion || len(declaration.Permissions) != 0 {
		p.mu.Unlock()
		return nil, errors.New("UI 视图未声明、版本不兼容或权限不受支持")
	}
	entry := declaration.Entry
	for session := range p.views {
		if !session.Active() {
			delete(p.views, session)
		}
	}
	count := len(p.views)
	epoch := p.viewEpoch
	p.mu.Unlock()
	for session := range m.views {
		if !session.Active() {
			delete(m.views, session)
		}
	}
	if count >= maxUIPluginSessions || len(m.views) >= maxUISessions {
		return nil, errors.New("UI 会话数量超过限制")
	}
	check := func() error {
		actual, err := packageFingerprint(p.dir)
		if err != nil || actual != p.fingerprint {
			p.markUIChanged()
			return errors.New("插件文件已变化，请先重载")
		}
		return nil
	}
	if err = check(); err != nil {
		return nil, err
	}
	bundle, err := uiview.LoadDirectory(ctx, filepath.Join(p.dir, "ui"), id, entry)
	if err != nil {
		return nil, err
	}
	if err = check(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	session, err := uiview.NewSession(uiview.Binding{PluginID: id, Fingerprint: p.fingerprint, ViewID: viewID, BridgeVersion: declaration.BridgeVersion}, bundle)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.removed || p.disabled || p.changed || epoch != p.viewEpoch {
		session.Revoke()
		return nil, errors.New("插件不可用")
	}
	if p.views == nil {
		p.views = map[*uiview.Session]struct{}{}
	}
	if m.views == nil {
		m.views = map[*uiview.Session]struct{}{}
	}
	p.views[session] = struct{}{}
	m.views[session] = struct{}{}
	return session, nil
}

func (p *plugin) revokeViewsLocked() {
	p.viewEpoch++
	for session := range p.views {
		session.Revoke()
	}
	p.views = nil
}

func (p *plugin) markUIChanged() { p.mu.Lock(); p.changed = true; p.revokeViewsLocked(); p.mu.Unlock() }
