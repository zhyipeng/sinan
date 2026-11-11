package main

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/platform"
	protocol "sinan/packages/plugin-protocol"
	"strconv"
)

func (h *HostService) ThemeSettings() (protocol.ThemeState, error) { return h.manager.ThemeSettings() }
func (h *HostService) SaveTheme(mode string, revision uint64) (protocol.ThemeState, error) {
	h.themeMu.Lock()
	defer h.themeMu.Unlock()
	state, err := h.manager.SaveTheme(mode, revision)
	if err != nil {
		return state, err
	}
	if h.ui != nil {
		h.ui.SetTheme(state)
	}
	if h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "theme:changed", Data: state, Sender: "kernel.theme"})
	}
	return state, nil
}
func themeColour(state protocol.ThemeState, appearance string) application.RGBA {
	value := state.Light.Tokens["background"]
	if appearance == "dark" {
		value = state.Dark.Tokens["background"]
	}
	number, _ := strconv.ParseUint(value[1:], 16, 32)
	return application.NewRGB(uint8(number>>16), uint8(number>>8), uint8(number))
}

// Appearance is window-local; system media changes never rewrite saved preferences.
func (h *HostService) ApplyThemeAppearance(ctx context.Context, revision uint64, appearance string) error {
	h.themeMu.Lock()
	defer h.themeMu.Unlock()
	state, err := h.manager.ThemeSettings()
	if err != nil {
		return err
	}
	if state.Revision != revision {
		return errors.New("主题设置已变化")
	}
	if appearance != "light" && appearance != "dark" || state.Mode != "system" && state.Mode != appearance {
		return errors.New("主题外观无效")
	}
	if h.launcher == nil {
		return errors.New("窗口不可用")
	}
	window := h.launcher.caller(ctx)
	if window == nil {
		return errors.New("窗口不可用")
	}
	application.InvokeSync(func() { platform.SetWindowTheme(window.NativeWindow(), state.Mode) })
	window.SetBackgroundColour(themeColour(state, appearance))
	return nil
}
