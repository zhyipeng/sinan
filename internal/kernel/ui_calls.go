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

func (m *Manager) callUIView(ctx context.Context, p *plugin, session *uiview.Session, view protocol.UIView, req protocol.UIRequest) (json.RawMessage, error) {
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
	if req.Method == "storage.get" || req.Method == "storage.put" {
		defer m.mutationMu.Unlock()
		if !slices.Contains(view.Permissions, "storage") {
			return nil, errors.New("此视图未声明私有数据权限")
		}
		if !m.uiAllowed(p, view.ID+"/storage") {
			return nil, errors.New("私有数据访问未授权，请在插件管理中设置 UI 权限")
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
	commandAllowed := m.uiAllowed(p, view.ID+"/command/"+req.CommandID)
	m.mutationMu.Unlock()
	if req.Method != "command.execute" || !slices.Contains(view.Commands, req.CommandID) {
		return nil, errors.New("此视图未声明该命令")
	}
	if !commandAllowed {
		return nil, errors.New("命令调用未授权，请在插件管理中设置 UI 权限")
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
