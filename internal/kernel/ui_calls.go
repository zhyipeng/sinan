package kernel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"sinan/internal/clipboard"
	"sinan/internal/paste"
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
	if req.Method == "dependency.status" || req.Method == "dependency.open" || req.Method == "dependency.manage" || strings.HasPrefix(req.Method, "image.upload.") {
		m.mutationMu.Unlock()
		return m.callDependency(ctx, p, session, view, req)
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
	if req.Method == "storage.encrypted.get" || req.Method == "storage.encrypted.put" {
		defer m.mutationMu.Unlock()
		if !slices.Contains(view.Permissions, "storage.encrypted") {
			return nil, errors.New("此视图未声明加密数据权限")
		}
		var result any
		if req.Method == "storage.encrypted.get" {
			result, err = m.encryptedDocuments.Get(p.manifest.ID)
		} else {
			result, err = m.encryptedDocuments.Put(p.manifest.ID, req.Revision, req.Value)
		}
		if err != nil {
			return nil, err
		}
		return marshalUIResult(result)
	}
	if strings.HasPrefix(req.Method, "llm.") {
		defer m.mutationMu.Unlock()
		if !slices.Contains(view.Permissions, "llm") {
			return nil, errors.New("此视图未声明 LLM 能力")
		}
		return m.callLLM(session, p.manifest.ID, req)
	}
	if req.Method == "clipboard.readImage" {
		defer m.mutationMu.Unlock()
		if !slices.Contains(view.Permissions, "clipboard.read") {
			return nil, errors.New("此视图未声明剪贴板读取权限")
		}
		value, err := clipboard.Call(ctx, "clipboard.read", protocol.ClipboardParams{})
		if err != nil {
			return nil, err
		}
		if value.Content.Kind != "image" {
			return marshalUIResult(nil)
		}
		return attachClipboardImage(session, value.Content)
	}
	if req.Method == "clipboard.readText" || req.Method == "clipboard.writeText" {
		defer m.mutationMu.Unlock()
		permission := "clipboard.read"
		method := "clipboard.read"
		params := protocol.ClipboardParams{}
		if req.Method == "clipboard.writeText" {
			permission = "clipboard.write"
			method = "clipboard.write"
			params.Content = protocol.ClipboardContent{Kind: "text", Data: []byte(req.Input)}
		}
		if !slices.Contains(view.Permissions, permission) {
			return nil, errors.New("此视图未声明剪贴板权限")
		}
		value, err := clipboard.Call(ctx, method, params)
		if err != nil {
			return nil, err
		}
		if method == "clipboard.write" {
			return marshalUIResult(nil)
		}
		if value.Content.Kind != "" && value.Content.Kind != "text" {
			return nil, errors.New("当前剪贴板不是文本")
		}
		if len(value.Content.Data) > 32<<10 {
			return nil, errors.New("剪贴板文本超过 UI 32 KiB 限制")
		}
		return marshalUIResult(string(value.Content.Data))
	}
	if req.Method == "input.paste" || req.Method == "window.execute" || req.Method == "window.show" || req.Method == "window.hide" || req.Method == "file.savePNG" || req.Method == "file.openImage" {
		handler := m.uiHostCall
		allowed := slices.Contains(view.Permissions, req.Method)
		if (req.Method == "window.execute" || req.Method == "file.savePNG") && !slices.Contains(view.Commands, req.CommandID) {
			allowed = false
		}
		m.mutationMu.Unlock()
		if !allowed || handler == nil {
			if req.Method == "window.execute" {
				return nil, errors.New("此视图未声明窗口执行权限、命令或宿主不支持")
			}
			return nil, errors.New("此视图未声明所需权限、命令或宿主不支持")
		}
		return handler(ctx, session, req)
	}
	m.mutationMu.Unlock()
	if (req.Method != "command.execute" && req.Method != "command.executeImage") || !slices.Contains(view.Commands, req.CommandID) {
		return nil, errors.New("此视图未声明该命令")
	}
	params := protocol.CommandParams{CommandID: req.CommandID, Input: req.Input}
	if req.Method == "command.executeImage" {
		var ok bool
		params.Image, ok = session.Image(req.Input)
		if !ok {
			return nil, errors.New("图片不属于当前会话或已失效")
		}
		params.Input = ""
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
	if err = process.Call(ctx, req.Method, params, &result); err != nil {
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

// Separate validation allows tests without accessing the system clipboard.
func attachClipboardImage(session *uiview.Session, content protocol.ClipboardContent) (json.RawMessage, error) {
	if content.Kind != "image" {
		return nil, errors.New("请先截图或复制一张图片")
	}
	parsed, err := paste.Parse(protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(content.Data)}})
	if err != nil {
		return nil, err
	}
	data, width, height := parsed.Image()
	image := session.AttachLaunchImage(data, width, height)
	if image.URL == "" {
		return nil, errors.New("图片会话已失效")
	}
	return marshalUIResult(image)
}

// ExecuteViewCommand is host-only: identity, package fingerprint and allowed
// command are rechecked against the original session before dispatch.
func (m *Manager) ExecuteViewCommand(ctx context.Context, session *uiview.Session, commandID, input string) (json.RawMessage, error) {
	if len(input) > uiview.MaxCallBytes {
		return nil, errors.New("命令参数过长")
	}
	binding := session.Binding()
	p, err := m.lookup(binding.PluginID)
	if err != nil {
		return nil, err
	}
	if p.fingerprint != binding.Fingerprint {
		return nil, errors.New("插件会话已替换")
	}
	for _, view := range p.manifest.Contributes.UIViews {
		if view.ID == binding.ViewID {
			return m.callUIView(ctx, p, session, view, protocol.UIRequest{Method: "command.execute", CommandID: commandID, Input: input}, protocol.UILaunchContext{})
		}
	}
	return nil, errors.New("插件会话不可用")
}
