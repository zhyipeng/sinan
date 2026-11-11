package main

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenSettings is available only to a registered trusted Shell window.
func (h *HostService) OpenSettings(ctx context.Context) error {
	if h.launcher == nil || h.launcher.caller(ctx) == nil {
		return errors.New("窗口不可用")
	}
	return h.openSettings()
}

func (h *HostService) openSettings() error {
	if _, _, err := trustedView(h.manager.Snapshot(), "system.settings", "open"); err != nil {
		return errors.New("设置插件不可用")
	}
	if h.launcher == nil {
		return errors.New("窗口不可用")
	}
	// Reuse detached settings so its in-progress edits remain intact.
	h.launcher.mu.Lock()
	for _, entry := range h.launcher.detached {
		if entry.key == "system.settings/settings" {
			entry.window.Show()
			entry.window.Focus()
			h.launcher.mu.Unlock()
			return nil
		}
	}
	h.launcher.mu.Unlock()
	h.launcher.mu.Lock()
	main := h.launcher.window
	h.launcher.mu.Unlock()
	h.releaseEmbedded(main)
	if h.ui != nil {
		h.ui.ReleaseMain()
	}
	h.launcher.mu.Lock()
	defer h.launcher.mu.Unlock()
	h.launcher.showLocked()
	h.launcher.window.DispatchWailsEvent(&application.CustomEvent{Name: "settings:open", Sender: "kernel.settings"})
	return nil
}
