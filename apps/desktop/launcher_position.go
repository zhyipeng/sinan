package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"sinan/internal/kernel"
)

func (h *HostService) LauncherPositionSettings() (kernel.LauncherPositionPreferences, error) {
	return h.manager.LauncherPositionPreferences()
}

func (h *HostService) SaveLauncherPosition(percent int, revision uint64) (kernel.LauncherPositionPreferences, error) {
	state, err := h.manager.SaveLauncherPositionPreferences(percent, revision)
	if err == nil && h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "launcher-position:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, err
}
