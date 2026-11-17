package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"sinan/internal/inputmatch"
	"sinan/internal/paste"
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
	return m.openUIViewLocked(ctx, id, viewID, protocol.UILaunchContext{})
}

// OpenUICommand resolves the current installed command and opens its view under
// one mutation lock, so reload/upgrade cannot replace the binding in between.
func (m *Manager) OpenUICommand(ctx context.Context, id, commandID string) (*uiview.Session, error) {
	return m.openUICommand(ctx, id, commandID, nil)
}

// OpenUICommandWithInput binds input to a newly prepared view, never global state.
func (m *Manager) OpenUICommandWithInput(ctx context.Context, id, commandID, input string) (*uiview.Session, error) {
	if len(input) > inputmatch.MaxInputBytes || !utf8.ValidString(input) {
		return nil, errors.New("打开参数无效或超过 64 KiB")
	}
	encoded, err := marshalUIResult(protocol.UILaunchContext{CommandID: commandID, Input: &input})
	if err != nil || len(encoded) > uiview.MaxCallResultBytes {
		return nil, errors.New("打开参数编码后超过 UI 返回预算")
	}
	return m.openUICommand(ctx, id, commandID, &input)
}
func (m *Manager) openUICommand(ctx context.Context, id, commandID string, input *string) (*uiview.Session, error) {
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
			if input != nil {
				for _, view := range p.manifest.Contributes.UIViews {
					if view.ID == command.UIView && view.BridgeVersion != protocol.UIRequestBridgeVersion {
						return nil, errors.New("打开参数需要 UI 桥 2")
					}
				}
			}
			return m.openUIViewLocked(ctx, id, command.UIView, protocol.UILaunchContext{CommandID: commandID, Input: input})
		}
	}
	return nil, errors.New("命令不存在或未声明独立视图")
}

func (m *Manager) openUIViewLocked(ctx context.Context, id, viewID string, launch protocol.UILaunchContext, images ...*paste.Content) (*uiview.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := m.lookup(id)
	if err != nil {
		return nil, err
	}
	if err := m.checkRequired(p, map[string]bool{}); err != nil {
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
	if declaration == nil || !protocol.SupportedUIBridge(declaration.BridgeVersion) {
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
	headerTitle, headerSource := uiHeaderTitle(p.manifest.Name, declaration.Title)
	session, err := uiview.NewSession(uiview.Binding{HeaderTitle: headerTitle, HeaderSource: headerSource, PluginID: id, Title: uiWindowTitle(p.manifest.Name, declaration.Title), Fingerprint: p.fingerprint, ViewID: viewID, BridgeVersion: declaration.BridgeVersion}, bundle)
	if err != nil {
		return nil, err
	}
	if len(images) > 0 && images[0] != nil && images[0].IsImage() {
		data, width, height := images[0].Image()
		image := session.AttachLaunchImage(data, width, height)
		launch.Image = &image
	}
	session.ConfigureCaller(func(ctx context.Context, req protocol.UIRequest) (json.RawMessage, error) {
		return m.callUIView(ctx, p, session, *declaration, req, launch)
	})
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

func uiWindowTitle(name, title string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 64 {
		runes = append(runes[:64], '…')
	}
	name = string(runes)
	if name == "" {
		name = "司南插件"
	}
	if title = strings.TrimSpace(title); title != "" {
		return name + " · " + title
	}
	return name
}

// The host owns header text; third-party HTML cannot replace navigation chrome.
func uiHeaderTitle(name, title string) (string, string) {
	name = uiWindowTitle(name, "")
	title = strings.TrimSpace(title)
	if title != "" && title != name {
		return title, name
	}
	return name, ""
}

// OpenUICommandWithPaste rechecks the current declaration before transferring
// an explicit paste into a view, including after plugin reload or upgrade.
func (m *Manager) OpenUICommandWithPaste(ctx context.Context, id, commandID string, content *paste.Content) (*uiview.Session, error) {
	return m.openImageCommand(ctx, id, commandID, content, false)
}
func (m *Manager) PrepareImageCommand(ctx context.Context, id, commandID string, content *paste.Content) (*uiview.Session, error) {
	return m.openImageCommand(ctx, id, commandID, content, true)
}
func (m *Manager) openImageCommand(ctx context.Context, id, commandID string, content *paste.Content, suspended bool) (*uiview.Session, error) {
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
		if command.ID != commandID || command.UIView == "" || content == nil || !content.Matches(command.InputMatchers) {
			continue
		}
		launch := protocol.UILaunchContext{CommandID: commandID}
		if text, ok := content.Text(); ok {
			launch.Input = &text
		}
		encoded, err := marshalUIResult(launch)
		if err != nil || len(encoded) > uiview.MaxCallResultBytes {
			return nil, errors.New("粘贴参数编码后超过 UI 返回预算")
		}
		for _, view := range p.manifest.Contributes.UIViews {
			if view.ID == command.UIView && view.BridgeVersion != protocol.UIRequestBridgeVersion {
				return nil, errors.New("粘贴输入需要 UI 桥 2")
			}
		}
		s, err := m.openUIViewLocked(ctx, id, command.UIView, launch, content)
		if err == nil && suspended {
			s.Suspend()
		}
		return s, err
	}
	return nil, errors.New("此插件命令不匹配当前粘贴内容")
}
