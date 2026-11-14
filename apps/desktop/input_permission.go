package main

import (
	"errors"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/platform"
)

// Queue a single guide without blocking the bounded plugin bridge request.
// Scheduling and presentation are injected so tests never create native UI.
type accessibilityGuide struct{ pending atomic.Bool }

func (g *accessibilityGuide) request(err error, valid func() bool, schedule func(func()), show func()) {
	if !errors.Is(err, platform.ErrAccessibilityPermission) || !g.pending.CompareAndSwap(false, true) {
		return
	}
	schedule(func() {
		defer g.pending.Store(false)
		if valid() {
			show()
		}
	})
}
func (h *HostService) guideInputPermission(err error, entry *embeddedEntry) {
	h.inputPermissionGuide.request(err, func() bool {
		h.embedded.mu.Lock()
		defer h.embedded.mu.Unlock()
		return h.embedded.entries[entry.window] == entry && entry.session.Active()
	}, application.InvokeAsync, func() {
		done := h.launcher.beginDialog(entry.window)
		defer done()
		platform.ShowAccessibilityPermissionGuide()
	})
}
