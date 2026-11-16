package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
)

func (h *HostService) SearchPreferences() (kernel.SearchPreferences, error) {
	return h.manager.SearchPreferences()
}

func (h *HostService) SaveSearchPreferences(spaceToOpen bool, revision uint64) (kernel.SearchPreferences, error) {
	state, err := h.manager.SaveSearchPreferences(spaceToOpen, revision)
	if err == nil && h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "search-preferences:changed", Data: state, Sender: "kernel.settings"})
	}
	return state, err
}
