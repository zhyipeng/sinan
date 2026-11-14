package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func (m *Manager) callUIView(ctx context.Context, p *plugin, session *uiview.Session, view protocol.UIView, req protocol.UIRequest, launch protocol.UILaunchContext) (json.RawMessage, error) {
	if err := m.beginOperation(); err != nil {
		return nil, err
	}
	defer m.active.Done()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Mutations cannot replace the instance while admitting a call or saving data.
	if !m.mutationMu.TryLock() {
		return nil, errors.New("插件正在变更，请稍后重试")
	}
	if !p.mu.TryLock() {
		m.mutationMu.Unlock()
		return nil, errors.New("插件正在切换，请稍后重试")
	}
	_, owned := p.views[session]
	valid := owned && !p.removed && !p.changed && !p.disabled && session.Active()
	p.mu.Unlock()
	if !valid {
		m.mutationMu.Unlock()
		return nil, errors.New("UI 会话不可用")
	}
	actual, err := packageFingerprint(p.dir)
	if err != nil || actual != p.fingerprint {
		p.markUIChanged()
		m.mutationMu.Unlock()
		return nil, errors.New("插件包已变化，请先重载")
	}
	if err = ctx.Err(); err != nil {
		m.mutationMu.Unlock()
		return nil, err
	}
	if req.Method == "launch.get" {
		defer m.mutationMu.Unlock()
		return marshalUIResult(launch)
	}
	if req.Method == "storage.get" || req.Method == "storage.put" {
		defer m.mutationMu.Unlock()
		if !slices.Contains(view.Permissions, "storage") {
			return nil, errors.New("此视图未声明私有数据权限")
		}
		var result any
		if req.Method == "storage.get" {
			result, err = m.documents.Get(p.manifest.ID)
		} else {
			result, err = m.documents.Put(p.manifest.ID, req.Revision, req.Value)
		}
		if err != nil {
			return nil, err
		}
		return marshalUIResult(result)
	}
	if req.Method == "input.paste" {
		handler := m.uiHostCall
		allowed := slices.Contains(view.Permissions, "input.paste")
		m.mutationMu.Unlock()
		if !allowed || handler == nil {
			return nil, errors.New("此视图未声明输入权限或平台不支持")
		}
		return handler(ctx, session, req)
	}
	m.mutationMu.Unlock()
	if req.Method != "command.execute" || !slices.Contains(view.Commands, req.CommandID) {
		return nil, errors.New("此视图未声明该命令")
	}
	// Calls route to the captured plugin instance; never resolve a replacement by ID.
	process, err := p.ensureUIProcess(ctx, m.root)
	if err != nil {
		return nil, err
	}
	if !session.Active() {
		return nil, errors.New("UI 会话已撤销")
	}
	var result protocol.CommandResult
	if err = process.Call(ctx, "command.execute", protocol.CommandParams{CommandID: req.CommandID, Input: req.Input}, &result); err != nil {
		return nil, errors.New("插件命令失败或超时")
	}
	// UI calls do not dismiss search or add search usage history.
	return marshalUIResult(result)
}

func marshalUIResult(value any) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// SetUIHostCall is configured once before serving sessions; never exposed to pages.
func (m *Manager) SetUIHostCall(handler func(context.Context, *uiview.Session, protocol.UIRequest) (json.RawMessage, error)) {
	m.uiHostCall = handler
}

// ActivateStartup starts only an enabled, explicitly declared background process.
func (m *Manager) ActivateStartup(ctx context.Context, id string) error {
	if err := m.beginOperation(); err != nil {
		return err
	}
	defer m.active.Done()
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(id)
	if err != nil {
		return err
	}
	if !slices.Contains(p.manifest.ActivationEvents, "onStartup") {
		return errors.New("未声明后台激活")
	}
	_, err = p.ensureProcess(ctx, m.root)
	return err
}
