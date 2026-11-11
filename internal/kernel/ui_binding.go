package kernel

import (
	"context"
	"errors"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

// ResolveUIView validates current package metadata without allocating a view
// slot. Reusing an existing window must work even when all slots are occupied.
func (m *Manager) ResolveUIView(ctx context.Context, id, viewID string) (uiview.Binding, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return m.resolveUIViewLocked(ctx, id, viewID)
}
func (m *Manager) ResolveUICommand(ctx context.Context, id, commandID string) (uiview.Binding, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(id)
	if err != nil {
		return uiview.Binding{}, err
	}
	for _, command := range p.manifest.Contributes.Commands {
		if command.ID == commandID && command.UIView != "" {
			return m.resolveUIViewLocked(ctx, id, command.UIView)
		}
	}
	return uiview.Binding{}, errors.New("命令不存在或未声明独立视图")
}
func (m *Manager) resolveUIViewLocked(ctx context.Context, id, viewID string) (uiview.Binding, error) {
	if err := ctx.Err(); err != nil {
		return uiview.Binding{}, err
	}
	p, err := m.lookup(id)
	if err != nil {
		return uiview.Binding{}, err
	}
	p.mu.Lock()
	unavailable := p.removed || p.disabled || p.changed
	p.mu.Unlock()
	if unavailable {
		return uiview.Binding{}, errors.New("插件不可用，请启用或重载后重试")
	}
	var binding uiview.Binding
	for _, view := range p.manifest.Contributes.UIViews {
		if view.ID == viewID && protocol.SupportedUIBridge(view.BridgeVersion) {
			headerTitle, headerSource := uiHeaderTitle(p.manifest.Name, view.Title)
			binding = uiview.Binding{HeaderTitle: headerTitle, HeaderSource: headerSource, PluginID: id, Fingerprint: p.fingerprint, ViewID: viewID, BridgeVersion: view.BridgeVersion, Title: uiWindowTitle(p.manifest.Name, view.Title)}
			break
		}
	}
	if binding.ViewID == "" {
		return binding, errors.New("UI 视图未声明")
	}
	actual, err := packageFingerprint(p.dir)
	if err != nil || actual != p.fingerprint {
		p.markUIChanged()
		return uiview.Binding{}, errors.New("插件文件已变化，请先重载")
	}
	if err = ctx.Err(); err != nil {
		return uiview.Binding{}, err
	}
	return binding, nil
}
