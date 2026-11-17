package main

import (
	"errors"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
)

// Only l.mu accesses the session clock. It never stores input or plugin content.
type mainRetention struct {
	preferences  kernel.RetentionPreferences
	hiddenAt     time.Time
	epoch        uint64
	inputCleared bool
	timer        *time.Timer
	closed       bool
}
type mainResume struct {
	PreserveInput bool   `json:"preserveInput"`
	PreserveView  bool   `json:"preserveView"`
	Epoch         uint64 `json:"epoch"`
}

func (r *mainRetention) decision(now time.Time) mainResume {
	state := mainResume{Epoch: r.epoch}
	if r.hiddenAt.IsZero() {
		return state
	}
	elapsed := now.Sub(r.hiddenAt)
	state.PreserveInput = !r.inputCleared && elapsed < time.Duration(r.preferences.ClearInputSeconds)*time.Second
	state.PreserveView = elapsed < time.Duration(r.preferences.PluginUISeconds)*time.Second
	return state
}
func (r *mainRetention) stopTimer() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}
func (r *mainRetention) shown() { r.stopTimer(); r.hiddenAt = time.Time{}; r.epoch++ }

func (r *mainRetention) hidden(now time.Time) bool {
	if !r.hiddenAt.IsZero() || r.closed {
		return false
	}
	r.hiddenAt = now
	r.epoch++
	r.inputCleared = false
	return true
}
func (r *mainRetention) expireInput(epoch uint64) bool {
	if r.closed || r.hiddenAt.IsZero() || r.epoch != epoch || r.inputCleared {
		return false
	}
	r.inputCleared = true
	return true
}

func (l *launcher) scheduleInputClearLocked() {
	r := &l.retention
	r.stopTimer()
	if r.closed || r.hiddenAt.IsZero() || r.inputCleared {
		return
	}
	// A stopped timer may already be waiting for l.mu; invalidate its callback.
	r.epoch++
	remaining := time.Until(r.hiddenAt.Add(time.Duration(r.preferences.ClearInputSeconds) * time.Second))
	epoch := r.epoch
	window := l.window
	clear := func() {
		window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:clear-input", Data: map[string]uint64{"epoch": epoch}, Sender: "kernel.launcher"})
	}
	if remaining <= 0 {
		r.expireInput(epoch)
		clear()
		return
	}
	r.timer = time.AfterFunc(remaining, func() {
		l.mu.Lock()
		if l.window != window || !r.expireInput(epoch) {
			l.mu.Unlock()
			return
		}
		r.timer = nil
		l.mu.Unlock()
		clear()
	})
}
func (l *launcher) hideLocked() {
	// Repeated hide requests must not extend either deadline.
	started := l.retention.hidden(time.Now())
	l.window.Hide()
	if started {
		l.scheduleInputClearLocked()
	}
}
func (l *launcher) reveal(window *application.WebviewWindow) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if window == l.window {
		cleared := l.retention.inputCleared
		l.retention.shown()
		window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:visibility", Data: map[string]any{"epoch": l.retention.epoch, "clearInput": cleared}, Sender: "kernel.launcher"})
	}
	window.Show()
}
func (l *launcher) closeRetention() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.retention.closed = true
	l.retention.shown()
}

func (h *HostService) RetentionSettings() (kernel.RetentionPreferences, error) {
	return h.manager.RetentionPreferences()
}
func (h *HostService) SaveRetentionSettings(clearInputSeconds, pluginUISeconds int, revision uint64) (kernel.RetentionPreferences, error) {
	h.retentionMu.Lock()
	defer h.retentionMu.Unlock()
	if h.ui != nil && pluginUISeconds > 0 {
		return kernel.RetentionPreferences{}, errors.New("当前独立客体承载不支持插件 UI 定时保留，请使用同窗口承载构建")
	}
	state, err := h.manager.SaveRetentionPreferences(clearInputSeconds, pluginUISeconds, revision)
	if err != nil {
		return state, err
	}
	if h.launcher != nil {
		h.launcher.mu.Lock()
		h.launcher.retention.preferences = state
		h.launcher.scheduleInputClearLocked()
		h.launcher.mu.Unlock()
		h.launcher.dispatch(&application.CustomEvent{Name: "retention:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, nil
}

func (h *HostService) resumeMain(toggle bool) {
	h.mainPresentationMu.Lock()
	defer h.mainPresentationMu.Unlock()
	l := h.launcher
	l.mu.Lock()
	if l.dialogs[l.window] > 0 {
		l.mu.Unlock()
		return
	}
	if toggle && l.window.IsVisible() {
		l.hideLocked()
		l.mu.Unlock()
		return
	}
	window := l.window
	state := l.retention.decision(time.Now())
	l.retention.shown()
	state.Epoch = l.retention.epoch
	l.mu.Unlock()
	if state.PreserveView && h.embedded != nil {
		h.embedded.mu.Lock()
		entry := h.embedded.entries[window]
		if entry != nil && (!entry.session.Active() || entry.executing || entry.pasting || entry.awaitingTask) {
			state.PreserveView = false
		}
		h.embedded.mu.Unlock()
	}
	if !state.PreserveView {
		h.releaseEmbedded(window)
		if h.ui != nil {
			h.ui.ReleaseMain()
		}
	}
	if h.ui != nil {
		// Legacy standalone guests have no host visibility clock. Keep their
		// existing control path; supported desktop builds use embedded views.
		if handled, _ := h.ui.Main(toggle); handled {
			return
		}
	}
	l.mu.Lock()
	l.presentLocked(state)
	target := l.inputTarget
	l.mu.Unlock()
	if state.PreserveView && h.embedded != nil {
		h.embedded.mu.Lock()
		entry := h.embedded.entries[window]
		if entry != nil {
			entry.inputTarget = target
		}
		h.embedded.mu.Unlock()
		if entry != nil && entry.session.Active() {
			entry.view.Show()
		}
	}
}
