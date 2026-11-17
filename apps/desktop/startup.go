package main

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
)

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
		state.AutoStart, err = h.startup.Enabled()
		if err != nil {
			state.AutoStartError = err.Error()
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
	if err := h.startup.SetEnabled(enabled); err != nil {
		return StartupState{}, err
	}
	return h.publishStartup()
}
func (h *HostService) publishStartup() (StartupState, error) {
	state, err := h.startupState()
	if err == nil && h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "startup:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, err
}
