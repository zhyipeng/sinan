package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/platform"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

// The host registers declarations; plugin business never enters the Shell.
func (h *HostService) startPluginRuntime(registry shortcutRegistry) {
	h.startPluginRuntimeWithDispatch(registry, application.InvokeAsync)
}

func (h *HostService) startPluginRuntimeWithDispatch(registry shortcutRegistry, dispatch func(func())) {
	ctx, cancel := context.WithCancel(context.Background())
	registry = runtimeShortcutRegistry{ctx: ctx, registry: registry, dispatch: dispatch}
	h.pluginRuntimeCancel = cancel
	h.pluginRuntimeDone = make(chan struct{})
	go func() {
		defer close(h.pluginRuntimeDone)

		retry := map[string]time.Time{}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			diagnostics := []string{}
			snapshot := h.manager.Snapshot()
			for _, p := range snapshot.Plugins {
				if p.Disabled || p.Changed || p.NeedsTrust {
					continue
				}
				if slices.Contains(p.ActivationEvents, "onStartup") && !time.Now().Before(retry[p.ID]) {
					deadline, stop := context.WithTimeout(ctx, 2*time.Second)
					err := h.manager.ActivateStartup(deadline, p.ID)
					stop()
					if err != nil {
						retry[p.ID] = time.Now().Add(5 * time.Second)
						diagnostics = append(diagnostics, p.ID+"：后台启动失败，请检查原生信任与插件数据")
					}
				}
			}
			diagnostics = append(diagnostics, h.syncPluginShortcuts(snapshot.Plugins, registry)...)
			h.pluginRuntimeMu.Lock()
			h.pluginRuntimeErrors = diagnostics
			h.pluginRuntimeMu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// OnShutdown waits for this worker on the UI thread. Only native registry calls
// run there; coordination and its mutexes stay on the worker. Cancellation must
// release the worker even when the UI queue cannot run. Wails releases remaining
// bindings after OnShutdown, so the worker must not unregister them on exit.
type runtimeShortcutRegistry struct {
	ctx      context.Context
	registry shortcutRegistry
	dispatch func(func())
}

func (r runtimeShortcutRegistry) invoke(call func() error) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	result := make(chan error, 1)
	r.dispatch(func() {
		if err := r.ctx.Err(); err != nil {
			result <- err
			return
		}
		result <- call()
	})
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case err := <-result:
		return err
	}
}

func (r runtimeShortcutRegistry) Register(key string, callback func()) error {
	return r.invoke(func() error { return r.registry.Register(key, callback) })
}

func (r runtimeShortcutRegistry) Unregister(key string) error {
	return r.invoke(func() error { return r.registry.Unregister(key) })
}

func (h *HostService) openShortcutCommand(owner string) {
	if h.embedded == nil || h.launcher == nil {
		return
	}
	var id, command string
	for i, c := range owner {
		if c == '/' {
			id = owner[:i]
			command = owner[i+1:]
			break
		}
	}
	if id == "" {
		return
	}
	h.embedded.mu.Lock()
	busy := false
	for _, entry := range h.embedded.entries {
		if entry.session != nil && entry.session.Binding().PluginID == id && (entry.executing || entry.awaitingTask) {
			busy = true
		}
	}
	h.embedded.mu.Unlock()
	if busy {
		return
	}
	// Capture before showing the Shell. show() also captures targets for ordinary opens.
	if !h.commandStartsHidden(id, command) {
		h.launcher.show()
	}
	h.launcher.mu.Lock()
	window := h.launcher.window
	h.launcher.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), application.WindowKey, window), 4*time.Second)
	defer cancel()
	if err := h.openEmbedded(ctx, id, "", command); err != nil {
		window.DispatchWailsEvent(&application.CustomEvent{Name: "plugin-shortcut:error", Data: "插件快捷键打开失败，请从搜索重试", Sender: "kernel.shortcuts"})
	}
}

func (h *HostService) callInputHost(ctx context.Context, session *uiview.Session, req protocol.UIRequest) (json.RawMessage, error) {
	if req.Method == "dependency.open" || req.Method == "dependency.manage" || strings.HasPrefix(req.Method, "image.upload.") {
		return h.callDependencyHost(ctx, session, req)
	}
	if req.Method == "file.openImage" {
		return h.callOpenImage(ctx, session)
	}
	if req.Method == "file.savePNG" {
		return h.callSavePNG(ctx, session, req)
	}
	if req.Method == "window.show" || req.Method == "window.hide" {
		return h.callWindowVisibility(ctx, session, req)
	}
	if req.Method == "window.execute" {
		return h.callWindowCommand(ctx, session, req)
	}
	if req.Method != "input.paste" || h.embedded == nil {
		return nil, errors.New("输入接口不可用")
	}
	h.embedded.mu.Lock()
	var entry *embeddedEntry
	for _, candidate := range h.embedded.entries {
		if candidate.session == session {
			entry = candidate
			break
		}
	}
	if entry == nil || !session.Active() || entry.pasting || entry.executing {
		h.embedded.mu.Unlock()
		return nil, errors.New("当前视图不可输入或正在粘贴")
	}
	target := entry.inputTarget
	if err := platform.CheckPasteTarget(target); err != nil {
		h.embedded.mu.Unlock()
		h.guideInputPermission(err, entry)
		return nil, err
	}
	entry.pasting = true
	h.embedded.mu.Unlock()

	// Keep the session alive until the bridge reply has been delivered. Failures
	// return through the existing Promise so the plugin can offer manual paste.
	valid := func() bool {
		h.embedded.mu.Lock()
		defer h.embedded.mu.Unlock()
		return h.embedded.entries[entry.window] == entry && session.Active()
	}
	if !valid() {
		return nil, errors.New("输入会话已失效")
	}
	h.hideSessionWindow(entry.window)
	err := platform.ActivateInputTarget(target)
	if err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(180 * time.Millisecond):
		}
		if err == nil && (!valid() || ctx.Err() != nil) {
			err = errors.New("输入会话已失效或超时")
		}
		if err == nil {
			err = platform.SendPaste(target)
		}
	}
	if err != nil {
		h.embedded.mu.Lock()
		if h.embedded.entries[entry.window] == entry {
			entry.pasting = false
		}
		h.embedded.mu.Unlock()
		if valid() {
			h.launcher.reveal(entry.window)
			entry.window.Focus()
		}
		return nil, errors.New("自动粘贴失败，内容已复制，请手动粘贴")
	}
	go func() { time.Sleep(100 * time.Millisecond); entry.view.Close() }()
	return json.RawMessage("null"), nil
}
