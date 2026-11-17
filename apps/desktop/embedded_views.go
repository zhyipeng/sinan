package main

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/paste"
	"sinan/internal/plugin/uihost"
	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	"sync"
	"time"
	"unsafe"
)

type embeddedSurface interface {
	Show()
	Close()
	SetDetached()
	SetTheme(protocol.ThemeState)
	SetDetachShortcut(string, uint64)
	Done() <-chan struct{}
}
type embeddedEntry struct {
	inputTarget  uintptr
	pasting      bool
	executing    bool
	awaitingTask bool
	hiddenLaunch bool
	view         embeddedSurface
	session      *uiview.Session
	window       *application.WebviewWindow
}
type embeddedViews struct {
	mu       sync.Mutex
	entries  map[*application.WebviewWindow]*embeddedEntry
	sequence map[*application.WebviewWindow]uint64
}

func (h *HostService) openEmbedded(ctx context.Context, id, viewID, commandID string) error {
	return h.openEmbeddedWithInput(ctx, id, viewID, commandID, nil)
}
func (h *HostService) openEmbeddedWithInput(ctx context.Context, id, viewID, commandID string, input *string) error {
	return h.openEmbeddedPrepared(ctx, id, viewID, commandID, input, nil, nil)
}
func (h *HostService) openEmbeddedPrepared(ctx context.Context, id, viewID, commandID string, input *string, content *paste.Content, check func() bool) error {
	window := h.launcher.caller(ctx)
	if window == nil {
		return errors.New("窗口不可用")
	}
	hidden := h.commandStartsHidden(id, commandID)
	if hidden {
		h.hideCommandWindows(window)
	}
	state := h.embedded
	state.mu.Lock()
	state.sequence[window]++
	sequence := state.sequence[window]
	state.mu.Unlock()
	opened := false
	defer func() {
		if hidden && !opened {
			state.mu.Lock()
			current := state.sequence[window] == sequence
			state.mu.Unlock()
			if current && ctx.Err() == nil {
				window.Show()
				window.Focus()
			}
		}
	}()
	var session *uiview.Session
	var err error
	if content != nil {
		session, err = h.manager.OpenUICommandWithPaste(ctx, id, commandID, content)
	} else if commandID != "" {
		if input == nil {
			session, err = h.manager.OpenUICommand(ctx, id, commandID)
		} else {
			session, err = h.manager.OpenUICommandWithInput(ctx, id, commandID, *input)
		}
	} else {
		session, err = h.manager.OpenUIView(ctx, id, viewID)
	}
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			session.Revoke()
		}
	}()
	config, err := h.manager.ThemeSettings()
	if err != nil {
		config = theme.Builtins("system", 0)
	}
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Resolve native window identity on Wails' own UI thread.
	var entry *embeddedEntry
	var nativeView *uihost.Embedded
	nativeWindow := func() unsafe.Pointer {
		if h.launcher.caller(ctx) != window {
			return nil
		}
		return window.NativeWindow()
	}
	shortcut := defaultDetachShortcut()
	if h.detachShortcuts != nil {
		shortcut = h.detachShortcuts.State().Shortcut
	}
	nativeView, err = uihost.PrepareEmbedded(deadline, session, nativeWindow, config, shortcut, func(kind int) {
		if kind == 3 {
			go h.resetWindow(window)
			return
		}
		if kind == 2 {
			_ = h.openSettings()
			return
		}
		if kind == 1 {
			_ = h.DetachTrustedView(context.WithValue(context.Background(), application.WindowKey, window), id, commandID)
		}
	}, application.InvokeSync)
	if err != nil {
		return err
	}
	h.launcher.mu.Lock()
	target := h.launcher.inputTarget
	h.launcher.mu.Unlock()
	entry = &embeddedEntry{inputTarget: target, view: nativeView, session: session, window: window, awaitingTask: hidden, hiddenLaunch: hidden}
	state.mu.Lock()
	if state.sequence[window] != sequence || !session.Active() || check != nil && !check() {
		state.mu.Unlock()
		nativeView.Close()
		return errors.New("插件页面打开已取消")
	}
	previous := state.entries[window]
	state.entries[window] = entry
	state.mu.Unlock()
	if previous != nil {
		previous.view.Close()
	}
	if latest, err := h.manager.ThemeSettings(); err == nil {
		nativeView.SetTheme(latest)
	}
	if h.detachShortcuts != nil {
		latest := h.detachShortcuts.State()
		nativeView.SetDetachShortcut(latest.Active, latest.Revision)
	}
	if !h.launcher.isMain(window) {
		nativeView.SetDetached()
	}
	if !hidden {
		nativeView.Show()
	}
	binding := session.Binding()
	h.launcher.mu.Lock()
	if detached, ok := h.launcher.detached[window.ID()]; ok {
		detached.key = id + "/ui/" + binding.ViewID
		h.launcher.detached[window.ID()] = detached
	}
	h.launcher.mu.Unlock()
	committed = true
	opened = true
	if hidden {
		go h.recoverHiddenView(entry)
	}
	window.DispatchWailsEvent(&application.CustomEvent{Name: "plugin-view:open", Sender: "kernel.views", Data: map[string]string{"pluginId": id, "viewId": binding.ViewID, "commandId": commandID, "title": binding.HeaderTitle}})
	go func() {
		<-nativeView.Done()
		state.mu.Lock()
		current := state.entries[window] == entry
		if current {
			delete(state.entries, window)
		}
		state.mu.Unlock()
		if current {
			window.DispatchWailsEvent(&application.CustomEvent{Name: "plugin-view:closed", Sender: "kernel.views", Data: map[string]string{"pluginId": id}})
		}
	}()
	return nil
}
func (h *HostService) releaseEmbedded(window *application.WebviewWindow) {
	if h.embedded == nil || window == nil {
		return
	}
	state := h.embedded
	state.mu.Lock()
	state.sequence[window]++
	entry := state.entries[window]
	delete(state.entries, window)
	state.mu.Unlock()
	if entry != nil {
		entry.view.Close()
	}
}
func (h *HostService) closeEmbeddedAll() {
	if h.embedded == nil {
		return
	}
	state := h.embedded
	state.mu.Lock()
	entries := state.entries
	state.entries = map[*application.WebviewWindow]*embeddedEntry{}
	for window := range state.sequence {
		state.sequence[window]++
	}
	state.mu.Unlock()
	for _, entry := range entries {
		entry.view.Close()
	}
}
func (h *HostService) ClosePluginView(ctx context.Context) {
	if h.launcher != nil {
		h.releaseEmbedded(h.launcher.caller(ctx))
	}
}
func (h *HostService) setEmbeddedTheme(state protocol.ThemeState) {
	if h.embedded == nil {
		return
	}
	h.embedded.mu.Lock()
	entries := make([]*embeddedEntry, 0, len(h.embedded.entries))
	for _, entry := range h.embedded.entries {
		entries = append(entries, entry)
	}
	h.embedded.mu.Unlock()
	for _, entry := range entries {
		entry.view.SetTheme(state)
	}
}

func (h *HostService) setEmbeddedShortcut(shortcut string, revision uint64) {
	if h.embedded == nil {
		return
	}
	h.embedded.mu.Lock()
	entries := make([]*embeddedEntry, 0, len(h.embedded.entries))
	for _, entry := range h.embedded.entries {
		entries = append(entries, entry)
	}
	h.embedded.mu.Unlock()
	for _, entry := range entries {
		entry.view.SetDetachShortcut(shortcut, revision)
	}
}
