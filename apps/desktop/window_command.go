package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func (h *HostService) commandStartsHidden(id, command string) bool {
	if h.manager == nil || command == "" {
		return false
	}
	for _, p := range h.manager.Snapshot().Plugins {
		if p.ID == id {
			for _, c := range p.Commands {
				if c.ID == command {
					return c.StartHidden
				}
			}
		}
	}
	return false
}
func (h *HostService) hideCommandWindows(window *application.WebviewWindow) {
	if window != nil {
		window.Hide()
	}
	h.launcher.mu.Lock()
	main := h.launcher.window
	h.launcher.mu.Unlock()
	if main != nil && main != window {
		main.Hide()
	}
}

// Page loading may call before PrepareEmbedded commits. Wait without holding
// the UI thread or state lock; cancellation/revocation terminates this wait.
func (h *HostService) windowCommandEntry(ctx context.Context, session *uiview.Session) (*embeddedEntry, error) {
	if h.embedded == nil {
		return nil, errors.New("窗口任务不可用")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := time.NewTimer(4 * time.Second)
	defer ready.Stop()
	for {
		h.embedded.mu.Lock()
		for _, entry := range h.embedded.entries {
			if entry.session == session {
				if entry.executing || entry.pasting {
					h.embedded.mu.Unlock()
					return nil, errors.New("当前窗口已有任务")
				}
				entry.executing = true
				entry.awaitingTask = false
				h.embedded.mu.Unlock()
				return entry, nil
			}
		}
		h.embedded.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-session.Done():
			return nil, errors.New("插件会话已撤销")
		case <-ready.C:
			return nil, errors.New("窗口尚未就绪")
		case <-ticker.C:
		}
	}
}
func (h *HostService) callWindowCommand(ctx context.Context, session *uiview.Session, req protocol.UIRequest) (json.RawMessage, error) {
	entry, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	valid := func() bool {
		h.embedded.mu.Lock()
		defer h.embedded.mu.Unlock()
		return h.embedded.entries[entry.window] == entry && session.Active()
	}
	dismiss := entry.hiddenLaunch
	keepHidden := false
	return runWindowCommand(ctx, valid, func() { h.hideCommandWindows(entry.window) }, func() (json.RawMessage, error) {
		value, err := h.manager.ExecuteViewCommand(ctx, session, req.CommandID, req.Input)
		dismiss = windowCommandDismissed(value, err, entry.hiddenLaunch)
		var result protocol.CommandResult
		if err == nil && json.Unmarshal(value, &result) == nil {
			keepHidden = result.KeepHidden
		}
		return value, err
	}, func() {
		if dismiss {
			if h.discardWindowCommand(entry) {
				h.hideCommandWindows(entry.window)
				if h.launcher.isMain(entry.window) {
					entry.window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:reset", Sender: "kernel.views"})
				} else {
					entry.window.Close()
				}
			}
			return
		}
		h.finishWindowCommand(entry, func() {
			if !keepHidden {
				entry.view.Show()
				entry.window.Show()
				entry.window.Focus()
			}
		})
	})
}

// The native compositor needs a brief settling interval after Hide, before a
// plugin freezes desktop pixels. Cancellation and replacement are checked again.
func runWindowCommand(ctx context.Context, valid func() bool, hide func(), execute func() (json.RawMessage, error), finish func()) (json.RawMessage, error) {
	defer finish()
	if !valid() {
		return nil, errors.New("插件会话已撤销")
	}
	hide()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(180 * time.Millisecond):
	}
	if !valid() {
		return nil, errors.New("插件会话已撤销")
	}
	return execute()
}
func (h *HostService) recoverHiddenView(entry *embeddedEntry) {
	select {
	case <-entry.session.Done():
		return
	case <-time.After(5 * time.Second):
	}
	h.embedded.mu.Lock()
	recover := h.embedded.entries[entry.window] == entry && entry.awaitingTask && entry.session.Active()
	entry.awaitingTask = false
	h.embedded.mu.Unlock()
	if recover {
		// A hidden launch that never starts must not open an empty editor.
		if h.discardWindowCommand(entry) {
			h.hideCommandWindows(entry.window)
			entry.window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:reset", Sender: "kernel.views"})
		}
	}
}

func (h *HostService) finishWindowCommand(entry *embeddedEntry, show func()) {
	h.embedded.mu.Lock()
	current := h.embedded.entries[entry.window] == entry
	entry.executing = false
	h.embedded.mu.Unlock()
	if current && entry.session.Active() {
		show()
	}
}

// Dismiss is the generic command result flag; the host never inspects plugin
// business data in Text to decide whether to present a window.
func windowCommandDismissed(value json.RawMessage, err error, hiddenLaunch bool) bool {
	if err != nil {
		return hiddenLaunch
	}
	var result protocol.CommandResult
	return json.Unmarshal(value, &result) == nil && result.Dismiss
}
func (h *HostService) discardWindowCommand(entry *embeddedEntry) bool {
	h.embedded.mu.Lock()
	current := h.embedded.entries[entry.window] == entry && entry.session.Active()
	entry.executing = false
	if current {
		delete(h.embedded.entries, entry.window)
		h.embedded.sequence[entry.window]++
	}
	h.embedded.mu.Unlock()
	if current {
		entry.session.Revoke()
		entry.view.Close()
	}
	return current
}
