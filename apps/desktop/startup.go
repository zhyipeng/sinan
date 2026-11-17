package main

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
)

// A narrow host-only interface allows headless tests without touching login items.
// The desktop injects app.Autostart after creating the Wails application.
type autostartService interface {
	IsEnabled() (bool, error)
	EnableWithOptions(application.AutostartOptions) error
	Disable() error
}

const autostartIdentifier = "dev.sinan.desktop"

func autostartError(err error, fallback string) string {
	if errors.Is(err, application.ErrAutostartNotSupported) {
		return "当前平台不支持开机自启"
	}
	return fallback
}

type StartupState struct {
	kernel.StartupPreferences
	AutoStart      bool   `json:"autoStart"`
	AutoStartError string `json:"autoStartError"`
}

func (h *HostService) StartupSettings() (StartupState, error) {
	h.startupMu.Lock()
	defer h.startupMu.Unlock()
	return h.startupState()
}
func (h *HostService) startupState() (StartupState, error) {
	prefs, err := h.manager.StartupPreferences()
	state := StartupState{StartupPreferences: prefs, AutoStartError: h.startupError}
	if err != nil {
		return state, err
	}
	if h.startup != nil {
		state.AutoStart, err = h.startup.IsEnabled()
		if err != nil {
			state.AutoStartError = autostartError(err, "无法读取开机自启配置")
		}
	}
	return state, nil
}
func (h *HostService) SaveSilentStartup(enabled bool, revision uint64) (StartupState, error) {
	h.startupMu.Lock()
	defer h.startupMu.Unlock()
	if _, err := h.manager.SaveStartupPreferences(enabled, revision); err != nil {
		return StartupState{}, err
	}
	return h.publishStartup()
}
func (h *HostService) SaveAutoStart(enabled bool) (StartupState, error) {
	h.startupMu.Lock()
	defer h.startupMu.Unlock()
	if h.startup == nil {
		return StartupState{}, errors.New("当前实例无法配置开机自启")
	}
	current, err := h.startup.IsEnabled()
	if err != nil {
		return StartupState{}, errors.New(autostartError(err, "无法读取开机自启配置"))
	}
	if enabled {
		// Keep an existing legacy registration as-is; enabling a second macOS
		// strategy alongside a LaunchAgent could start two app instances.
		if !current {
			err = h.startup.EnableWithOptions(application.AutostartOptions{Identifier: autostartIdentifier})
		}
	} else {
		err = h.startup.Disable()
	}
	if err != nil {
		return StartupState{}, errors.New(autostartError(err, "无法修改开机自启，请检查系统登录项设置"))
	}
	state, err := h.startupState()
	if err != nil {
		return state, err
	}
	if state.AutoStartError != "" {
		return state, errors.New(state.AutoStartError)
	}
	if state.AutoStart != enabled {
		return state, errors.New("开机自启尚未生效，请在系统设置中检查登录项是否需要批准")
	}
	if h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "startup:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, nil
}
func (h *HostService) publishStartup() (StartupState, error) {
	state, err := h.startupState()
	if err == nil && h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "startup:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, err
}
