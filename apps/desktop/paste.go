package main

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/paste"
	"sinan/internal/search"
	protocol "sinan/packages/plugin-protocol"
)

type stagedPaste struct {
	id       string
	sequence uint64
	owner    *application.WebviewWindow
	content  *paste.Content
}

// No clipboard monitoring: only explicit Shell paste events reach this service.
func (h *HostService) StartPasteSearch(ctx context.Context, sequence uint64, value protocol.PasteContent) error {
	if h.launcher == nil {
		return errors.New("主输入窗口不可用")
	}
	window := h.launcher.caller(ctx)
	if !h.launcher.isMain(window) {
		return errors.New("仅主输入窗口可开始粘贴匹配")
	}
	h.searchMu.Lock()
	stale := sequence <= h.searchSequence || sequence <= h.pasteClearedThrough
	h.searchMu.Unlock()
	if stale {
		return nil
	}
	if !h.pasteParseMu.TryLock() {
		return errors.New("正在处理上一份粘贴内容，请稍后重试")
	}
	defer h.pasteParseMu.Unlock()
	content, err := paste.Parse(value)
	if err != nil {
		return err
	}
	selection := &stagedPaste{id: rand.Text(), sequence: sequence, owner: window, content: content}
	h.startQuery(ctx, sequence, selection, func(ctx context.Context, emit func(search.Update)) {
		coordinator := h.searchCoordinator()
		coordinator.QueryPaste(ctx, queryID(sequence), selection.id, content, emit)
	})
	return nil
}
func (h *HostService) ClearPaste(ctx context.Context, sequence uint64) {
	if h.launcher == nil {
		return
	}
	window := h.launcher.caller(ctx)
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	if !h.launcher.isMain(window) {
		return
	}
	h.pasteClearedThrough = max(h.pasteClearedThrough, sequence)
	if h.pasted != nil && h.pasted.owner == window && h.pasted.sequence <= sequence {
		h.pasted = nil
	}
}
func (h *HostService) OpenPasteCommand(ctx context.Context, pluginID, commandID, id string) (bool, error) {
	if h.launcher == nil {
		return false, errors.New("主输入窗口不可用")
	}
	window := h.launcher.caller(ctx)
	h.searchMu.Lock()
	selection := h.pasted
	valid := selection != nil && selection.owner == window && selection.id == id
	h.searchMu.Unlock()
	if !valid {
		return false, errors.New("粘贴内容已失效，请重新粘贴")
	}
	check := func() bool { h.searchMu.Lock(); defer h.searchMu.Unlock(); return h.pasted == selection }
	if h.embedded != nil {
		return true, h.openEmbeddedPrepared(ctx, pluginID, "", commandID, nil, selection.content, check)
	}
	if h.ui == nil {
		return false, h.pluginUIUnavailable()
	}
	return false, h.ui.OpenCommandWithPaste(pluginID, commandID, selection.content)
}

// ResetToSearch is trusted window navigation, never an HTML bridge capability.
func (h *HostService) ResetToSearch(ctx context.Context) {
	if h.launcher != nil {
		h.resetWindow(h.launcher.caller(ctx))
	}
}
func (h *HostService) resetWindow(window *application.WebviewWindow) {
	if window == nil || h.launcher == nil {
		return
	}
	h.searchMu.Lock()
	if h.searchCancel != nil {
		h.searchCancel()
	}
	h.pasted = nil
	h.searchMu.Unlock()
	h.stopRecording(window)
	h.releaseEmbedded(window)
	if !h.launcher.isMain(window) {
		window.Close()
		go h.showMain()
		return
	}
	window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:reset", Sender: "kernel.launcher"})
}
