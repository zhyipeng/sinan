package main

import (
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"sinan/internal/platform"
)

// Keep native changes and persistence ordered across all trusted settings views.
type windowOpacityController struct {
	mu      sync.Mutex
	manager *kernel.Manager
	apply   func(int) error
}

func (c *windowOpacityController) state() (kernel.WindowOpacityPreferences, error) {
	state, err := c.manager.WindowOpacityPreferences()
	if err == nil {
		if nativeErr := c.apply(state.Percent); nativeErr != nil && state.Error == "" {
			state.Error = nativeErr.Error()
		}
	}
	return state, err
}
func (h *HostService) WindowOpacitySettings() (kernel.WindowOpacityPreferences, error) {
	if h.opacity == nil {
		return kernel.WindowOpacityPreferences{}, errors.New("窗口不可用")
	}
	h.opacity.mu.Lock()
	defer h.opacity.mu.Unlock()
	return h.opacity.state()
}
func (h *HostService) SaveWindowOpacity(percent int, revision uint64) (kernel.WindowOpacityPreferences, error) {
	c := h.opacity
	if c == nil {
		return kernel.WindowOpacityPreferences{}, errors.New("窗口不可用")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	previous, err := c.manager.WindowOpacityPreferences()
	if err != nil {
		return previous, err
	}
	if previous.Error != "" {
		return previous, errors.New(previous.Error)
	}
	if percent < 30 || percent > 100 || revision != previous.Revision {
		return previous, errors.New("窗口透明度无效或设置已变化，请重新读取后重试")
	}
	if err := c.apply(percent); err != nil {
		return previous, err
	}
	state, err := c.manager.SaveWindowOpacityPreferences(percent, revision)
	if err != nil {
		if c.apply(previous.Percent) != nil {
			return previous, errors.New("透明度保存失败，窗口恢复失败，请重新呼出窗口后重试")
		}
		return previous, err
	}
	if h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "window-opacity:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, nil
}

func (l *launcher) setOpacity(percent int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window == nil {
		return errors.New("窗口不可用")
	}
	var err error
	application.InvokeSync(func() { err = platform.SetWindowOpacity(l.window.NativeWindow(), percent) })
	if err == nil {
		l.opacityPercent = percent
	}
	return err
}

func (h *HostService) restoreWindowOpacity(window *application.WebviewWindow) {
	if !h.launcher.isMain(window) {
		return
	}
	state, err := h.WindowOpacitySettings()
	if err == nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "window-opacity:changed", Data: state, Sender: "kernel.settings"})
	}
}
