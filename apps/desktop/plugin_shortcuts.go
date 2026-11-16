package main

import (
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"sinan/internal/storage"
	"sort"
)

type pluginShortcutPreferences struct {
	manager             *kernel.Manager
	pluginID, commandID string
}

func (p pluginShortcutPreferences) LauncherPreferences() (storage.SettingsSnapshot, error) {
	return p.manager.PluginShortcutPreferences(p.pluginID, p.commandID)
}
func (p pluginShortcutPreferences) SaveLauncherPreferences(revision uint64, value string) (storage.SettingsSnapshot, error) {
	return p.manager.SavePluginShortcutPreferences(p.pluginID, p.commandID, revision, value)
}

type PluginShortcutState struct {
	PluginID   string        `json:"pluginId"`
	CommandID  string        `json:"commandId"`
	PluginName string        `json:"pluginName"`
	Title      string        `json:"title"`
	State      ShortcutState `json:"state"`
}
type pluginShortcutEntry struct {
	info       PluginShortcutState
	controller *shortcutController
}

// All collection access and changes share shortcutConfigMu with host shortcuts.
func (h *HostService) syncPluginShortcuts(plugins []kernel.PluginInfo, registry shortcutRegistry) []string {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	if h.pluginShortcuts == nil {
		h.pluginShortcuts = map[string]*pluginShortcutEntry{}
	}
	desired := map[string]PluginShortcutState{}
	diagnostics := []string{}
	for _, p := range plugins {
		if p.Disabled || p.Changed || p.NeedsTrust {
			continue
		}
		for _, command := range p.Commands {
			if command.Shortcut == "" || command.UIView == "" {
				continue
			}
			fallback, err := normalizeShortcut(command.Shortcut)
			if err != nil {
				diagnostics = append(diagnostics, p.ID+"：全局快捷键格式无效")
				continue
			}
			desired[p.ID+"/"+command.ID] = PluginShortcutState{PluginID: p.ID, CommandID: command.ID, PluginName: p.Name, Title: command.Title, State: ShortcutState{Default: fallback}}
		}
	}
	changed := false
	for owner, entry := range h.pluginShortcuts {
		info, exists := desired[owner]
		if !exists || info.State.Default != entry.controller.State().Default {
			entry.controller.Stop()
			delete(h.pluginShortcuts, owner)
			changed = true
		}
	}
	owners := make([]string, 0, len(desired))
	for owner := range desired {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		info := desired[owner]
		entry := h.pluginShortcuts[owner]
		if entry == nil {
			controller := newShortcutControllerWithDefault(pluginShortcutPreferences{h.manager, info.PluginID, info.CommandID}, registry, func() { h.openShortcutCommand(owner) }, h.captureShortcut, func(state ShortcutState) {
				h.updateAltSpaceHotkey()
				h.dispatchPluginShortcut(&application.CustomEvent{Name: "plugin-shortcut:changed", Data: map[string]any{"pluginId": info.PluginID, "commandId": info.CommandID, "state": state}, Sender: "kernel.shortcuts"})
			}, info.State.Default)
			entry = &pluginShortcutEntry{info: info, controller: controller}
			h.pluginShortcuts[owner] = entry
			changed = true
		}
		if entry.info.PluginName != info.PluginName || entry.info.Title != info.Title {
			changed = true
		}
		entry.info.PluginName = info.PluginName
		entry.info.Title = info.Title
		state := entry.controller.State()
		if !state.Ready || state.Active == "" {
			// Retry an unavailable declaration without discarding the saved override.
			if state.Ready {
				entry.controller.mu.Lock()
				entry.controller.state.Error = ""
				entry.controller.mu.Unlock()
			}
			entry.controller.Stop()
			if err := shortcutConflict(state.Shortcut, h.detachShortcuts); err != nil {
				entry.controller.mu.Lock()
				entry.controller.state.Ready = true
				entry.controller.state.Error = "快捷键已用于窗口独立"
				entry.controller.mu.Unlock()
			} else {
				entry.controller.Start()
			}
		}
		if state = entry.controller.State(); state.Error != "" {
			diagnostics = append(diagnostics, fmt.Sprintf("%s：%s", info.PluginName, state.Error))
		}
	}
	if changed {
		h.dispatchPluginShortcut(&application.CustomEvent{Name: "plugin-shortcuts:changed", Sender: "kernel.shortcuts"})
	}
	return diagnostics
}
func (h *HostService) PluginShortcutSettings() []PluginShortcutState {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	result := []PluginShortcutState{}
	for _, entry := range h.pluginShortcuts {
		info := entry.info
		info.State = entry.controller.State()
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PluginID+"/"+result[i].CommandID < result[j].PluginID+"/"+result[j].CommandID
	})
	return result
}
func (h *HostService) pluginShortcutConflict(value, exclude string) error {
	normalized, err := normalizeShortcut(value)
	if err != nil {
		return err
	}
	for owner, entry := range h.pluginShortcuts {
		if owner == exclude {
			continue
		}
		state := entry.controller.State()
		if normalized == state.Shortcut || normalized == state.Active {
			return fmt.Errorf("快捷键已用于「%s」", entry.info.Title)
		}
	}
	return nil
}
func (h *HostService) SavePluginShortcut(pluginID, commandID, value string, revision uint64) (ShortcutState, error) {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	owner := pluginID + "/" + commandID
	entry := h.pluginShortcuts[owner]
	if entry == nil {
		return ShortcutState{}, errors.New("插件快捷键不可用，请刷新设置")
	}
	for _, other := range []*shortcutController{h.shortcuts, h.detachShortcuts} {
		if other == nil {
			continue
		}
		normalized, err := normalizeShortcut(value)
		if err != nil {
			return ShortcutState{}, err
		}
		state := other.State()
		if normalized == state.Shortcut || normalized == state.Active {
			return ShortcutState{}, errors.New("快捷键已用于司南呼出或窗口独立")
		}
	}
	if err := h.pluginShortcutConflict(value, owner); err != nil {
		return ShortcutState{}, err
	}
	return entry.controller.Save(value, revision)
}
func (h *HostService) recordPluginShortcuts(enabled bool) {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	for _, entry := range h.pluginShortcuts {
		if entry.controller.State().Ready {
			_ = entry.controller.Record(enabled)
		}
	}
}
func (h *HostService) stopPluginShortcuts() {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	for _, entry := range h.pluginShortcuts {
		entry.controller.Stop()
	}
}

func (h *HostService) dispatchPluginShortcut(event *application.CustomEvent) {
	if h.launcher != nil {
		h.launcher.dispatch(event)
	}
}
