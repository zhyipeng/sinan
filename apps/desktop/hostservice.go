package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"sinan/internal/plugin/uiwindows"
	"sinan/internal/search"
	"sinan/internal/storage"
	protocol "sinan/packages/plugin-protocol"
	"sinan/plugins/system-plugin-store/catalog"
	"strconv"
	"sync"
)

// HostService is bound only to the trusted Shell, never to third-party UI.
type HostService struct {
	storeMu          sync.Mutex
	store            *catalog.Service
	embedded         *embeddedViews
	themeMu          sync.Mutex
	ui               *uiwindows.Controller
	uiError          string
	shortcuts        *shortcutController
	detachShortcuts  *shortcutController
	shortcutConfigMu sync.Mutex
	probeEnabled     bool
	manager          *kernel.Manager
	launcher         *launcher
	recordMu         sync.Mutex
	recordingWindow  *application.WebviewWindow
	searchMu         sync.Mutex
	searchSequence   uint64
	searchCancel     context.CancelFunc
}

func (h *HostService) Snapshot() kernel.Snapshot { return h.uiDiagnostics(h.manager.Snapshot()) }
func (h *HostService) Refresh() kernel.Snapshot  { return h.uiDiagnostics(h.manager.Refresh()) }
func (h *HostService) uiDiagnostics(snapshot kernel.Snapshot) kernel.Snapshot {
	if h.uiError != "" {
		snapshot.Diagnostics = append(snapshot.Diagnostics, h.uiError)
	}
	return snapshot
}
func (h *HostService) Install(directory string, trustNative bool) (kernel.PluginInfo, error) {
	return h.manager.Install(directory, trustNative)
}
func (h *HostService) Execute(pluginID, commandID, input string) (protocol.CommandResult, error) {
	return h.manager.Execute(context.Background(), pluginID, commandID, input)
}
func (h *HostService) Stop(pluginID string) error { return h.manager.Stop(pluginID) }
func (h *HostService) Uninstall(pluginID string) error {
	err := h.manager.Uninstall(pluginID)
	if err == nil && h.launcher != nil {
		h.launcher.closePlugin(pluginID)
	}
	return err
}

func (h *HostService) ResizeLauncher(ctx context.Context, height int) {
	if h.launcher != nil {
		h.launcher.resize(h.launcher.caller(ctx), height)
	}
}

func (h *HostService) HideLauncher(ctx context.Context) {
	if h.launcher != nil {
		h.launcher.hide(h.launcher.caller(ctx))
	}
}
func (h *HostService) LauncherStatus() string {
	if h.launcher != nil {
		return h.launcher.status()
	}
	return ""
}

// StartSearch belongs to the trusted Shell. Sequence numbers also reject
// out-of-order Wails invocations before they can supersede a newer query.
func (h *HostService) StartSearch(ctx context.Context, sequence uint64, query string) {
	if h.launcher == nil {
		return
	}
	window := h.launcher.caller(ctx)
	if !h.launcher.isMain(window) {
		return
	}
	h.searchMu.Lock()
	if sequence <= h.searchSequence {
		h.searchMu.Unlock()
		return
	}
	if h.searchCancel != nil {
		h.searchCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.searchSequence = sequence
	h.searchCancel = cancel
	h.searchMu.Unlock()
	coordinator := search.Coordinator{Backend: h.manager}
	go coordinator.Query(ctx, strconv.FormatUint(sequence, 10), query, func(update search.Update) {
		h.searchMu.Lock()
		defer h.searchMu.Unlock()
		if h.searchSequence == sequence && ctx.Err() == nil && h.launcher.isMain(window) {
			window.DispatchWailsEvent(&application.CustomEvent{Name: "search:update", Data: update, Sender: "kernel.search"})
		}
	})
}

func (h *HostService) CancelSearch(ctx context.Context, sequence uint64) {
	if h.launcher == nil || !h.launcher.isMain(h.launcher.caller(ctx)) {
		return
	}
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	if sequence == h.searchSequence && h.searchCancel != nil {
		h.searchCancel()
	}
}

func (h *HostService) cancelSearch() {
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	if h.searchCancel != nil {
		h.searchCancel()
	}
}

type IsolationReport struct {
	ParentBlocked   bool `json:"parentBlocked"`
	BindingsAbsent  bool `json:"bindingsAbsent"`
	StorageBlocked  bool `json:"storageBlocked"`
	RuntimeBlocked  bool `json:"runtimeBlocked"`
	ResourceBlocked bool `json:"resourceBlocked"`
	NetworkBlocked  bool `json:"networkBlocked"`
}

// Diagnostic output contains booleans only; no guest data or filesystem paths.
func (h *HostService) RecordIsolationProbe(report IsolationReport, rejected bool) {
	if !h.probeEnabled {
		return
	}
	data, _ := json.Marshal(struct {
		Report   IsolationReport `json:"report"`
		Rejected bool            `json:"invalidActionRejected"`
	}{report, rejected})
	fmt.Printf("ISOLATION_PROBE %s\n", data)
}

// Configuration methods are exposed only to the trusted Shell and bundled views.
func (h *HostService) PluginSettings(pluginID string) (storage.SettingsSnapshot, error) {
	return h.manager.PluginSettings(pluginID)
}
func (h *HostService) SavePluginSettings(pluginID string, revision uint64, values map[string][]string) (storage.SettingsSnapshot, error) {
	h.cancelSearch()
	result, err := h.manager.SavePluginSettings(pluginID, revision, values)
	if err == nil && h.launcher != nil {
		h.launcher.dispatch(&application.CustomEvent{Name: "settings:changed", Data: map[string]any{"pluginId": pluginID, "revision": result.Revision}, Sender: "kernel.settings"})
	}
	return result, err
}
func (h *HostService) ClearHistory() error { h.cancelSearch(); return h.manager.ClearUsage() }

func (h *HostService) PreviewPackage(directory string) (kernel.PackagePreview, error) {
	return h.manager.PreviewPackage(directory)
}
func (h *HostService) ApplyPackage(directory string, trustNative bool, fingerprint, installedFingerprint string) (kernel.PluginInfo, error) {
	h.cancelSearch()
	defer h.cancelSearch()
	result, err := h.manager.ApplyPackage(directory, trustNative, fingerprint, installedFingerprint)
	if err == nil && h.launcher != nil {
		h.launcher.closePlugin(result.ID)
	}
	return result, err
}
func (h *HostService) Reload(pluginID string) (kernel.PluginInfo, error) {
	h.cancelSearch()
	defer h.cancelSearch()
	result, err := h.manager.Reload(pluginID)
	if err == nil && h.launcher != nil {
		h.launcher.closePlugin(pluginID)
	}
	return result, err
}

func (h *HostService) ShortcutSettings() (ShortcutState, error) {
	if h.shortcuts == nil {
		return ShortcutState{}, fmt.Errorf("此运行模式不支持快捷键设置")
	}
	return h.shortcuts.State(), nil
}
func (h *HostService) SaveShortcut(value string, revision uint64) (ShortcutState, error) {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	if err := shortcutConflict(value, h.detachShortcuts); err != nil {
		return ShortcutState{}, err
	}
	if h.shortcuts == nil {
		return ShortcutState{}, fmt.Errorf("此运行模式不支持快捷键设置")
	}
	return h.shortcuts.Save(value, revision)
}
func (h *HostService) RecordShortcut(ctx context.Context, enabled bool) error {
	if h.shortcuts == nil || h.launcher == nil {
		return errors.New("此运行模式不支持快捷键录入")
	}
	window := h.launcher.caller(ctx)
	if window == nil {
		return errors.New("窗口不可用")
	}
	h.recordMu.Lock()
	defer h.recordMu.Unlock()
	if !enabled && h.recordingWindow != window {
		return nil
	}
	if enabled && (!window.IsVisible() || !window.IsFocused()) {
		return errors.New("窗口未聚焦，快捷键录入已取消")
	}
	if err := h.shortcuts.Record(enabled); err != nil {
		return err
	}
	if enabled {
		h.recordingWindow = window
		if !window.IsVisible() || !window.IsFocused() {
			h.recordingWindow = nil
			_ = h.shortcuts.Record(false)
			return errors.New("窗口未聚焦，快捷键录入已取消")
		}
	} else {
		h.recordingWindow = nil
	}
	return nil
}
func (h *HostService) stopRecording(window *application.WebviewWindow) {
	h.recordMu.Lock()
	defer h.recordMu.Unlock()
	if h.shortcuts != nil && h.recordingWindow == window {
		h.recordingWindow = nil
		_ = h.shortcuts.Record(false)
	}
}

func (h *HostService) SetPluginEnabled(pluginID string, enabled bool) (kernel.PluginInfo, error) {
	h.cancelSearch()
	defer h.cancelSearch()
	result, err := h.manager.SetPluginEnabled(pluginID, enabled)
	if err == nil && !enabled && h.launcher != nil {
		h.launcher.closePlugin(pluginID)
	}
	return result, err
}

// OpenPluginView is only bound to the trusted Shell; the child gets no Wails bridge.
func (h *HostService) OpenPluginView(ctx context.Context, pluginID, viewID string) error {
	if h.embedded != nil {
		return h.openEmbedded(ctx, pluginID, viewID, "")
	}
	if h.ui == nil {
		return h.pluginUIUnavailable()
	}
	return h.ui.Open(pluginID, viewID)
}
func (h *HostService) pluginUIUnavailable() error {
	if h.uiError != "" {
		return errors.New(h.uiError)
	}
	return errors.New("插件界面已关闭，请移除 -plugin-ui=false 后重启")
}

func (h *HostService) PluginUIEnabled() bool { return h.ui != nil || h.embedded != nil }

// OpenPluginCommand accepts command identity only; the kernel resolves its view.
func (h *HostService) OpenPluginCommand(ctx context.Context, pluginID, commandID string) (bool, error) {
	if h.embedded != nil {
		return true, h.openEmbedded(ctx, pluginID, "", commandID)
	}
	if h.ui == nil {
		return false, h.pluginUIUnavailable()
	}
	return false, h.ui.OpenCommand(pluginID, commandID)
}

func (h *HostService) toggleMain() {
	if h.embedded != nil {
		h.launcher.mu.Lock()
		window := h.launcher.window
		h.launcher.mu.Unlock()
		if !window.IsVisible() {
			h.releaseEmbedded(window)
		}
	}
	if h.ui != nil {
		if handled, _ := h.ui.Main(true); handled {
			return
		}
	}
	h.launcher.toggle()
}
func (h *HostService) showMain() {
	if h.embedded != nil {
		h.launcher.mu.Lock()
		window := h.launcher.window
		h.launcher.mu.Unlock()
		h.releaseEmbedded(window)
	}
	if h.ui != nil {
		if handled, _ := h.ui.Main(false); handled {
			return
		}
	}
	h.launcher.show()
}

// DetachTrustedView rotates the Shell role, preserving the original trusted WebView.
// Identity is resolved against installed commands and the bundled component allowlist.
func (h *HostService) DetachTrustedView(ctx context.Context, pluginID, commandID string) error {
	if h.launcher == nil {
		return errors.New("窗口不可用")
	}
	window := h.launcher.caller(ctx)
	if h.embedded != nil && window != nil {
		h.embedded.mu.Lock()
		entry := h.embedded.entries[window]
		h.embedded.mu.Unlock()
		if entry != nil && entry.session.Binding().PluginID == pluginID {
			binding, err := h.manager.ResolveUIView(ctx, pluginID, entry.session.Binding().ViewID)
			if err != nil || binding != entry.session.Binding() {
				return errors.New("插件页面已变化")
			}
			err = h.launcher.detach(window, binding.Title, pluginID+"/ui/"+binding.ViewID)
			if err == nil {
				entry.view.SetDetached()
				entry.view.Show()
			}
			return err
		}
	}
	title, key, err := trustedView(h.manager.Snapshot(), pluginID, commandID)
	if err != nil {
		return err
	}
	return h.launcher.detach(h.launcher.caller(ctx), title, key)
}
func trustedView(snapshot kernel.Snapshot, pluginID, commandID string) (string, string, error) {
	for _, plugin := range snapshot.Plugins {
		if plugin.ID != pluginID || plugin.Disabled {
			continue
		}
		for _, command := range plugin.Commands {
			key := plugin.ID + "/" + command.View
			if command.ID == commandID && (key == "system.settings/settings" || key == "system.plugin-manager/manager" || key == "system.plugin-store/store") {
				return command.Title, key, nil
			}
		}
	}
	return "", "", errors.New("此插件界面不支持拆出")
}
func (h *HostService) CloseTrustedView(ctx context.Context) {
	if h.launcher == nil {
		return
	}
	if window := h.launcher.caller(ctx); window != nil && !h.launcher.isMain(window) {
		window.Close()
	}
}

func shortcutConflict(value string, other *shortcutController) error {
	normalized, err := normalizeShortcut(value)
	if err != nil {
		return err
	}
	if other != nil {
		state := other.State()
		if normalized == state.Shortcut || normalized == state.Active {
			return errors.New("呼出与窗口独立快捷键不能相同")
		}
	}
	return nil
}
func (h *HostService) DetachShortcutSettings() (ShortcutState, error) {
	if h.detachShortcuts == nil {
		return ShortcutState{}, errors.New("窗口快捷键尚未就绪")
	}
	return h.detachShortcuts.State(), nil
}
func (h *HostService) SaveDetachShortcut(value string, revision uint64) (ShortcutState, error) {
	h.shortcutConfigMu.Lock()
	defer h.shortcutConfigMu.Unlock()
	if h.detachShortcuts == nil {
		return ShortcutState{}, errors.New("窗口快捷键尚未就绪")
	}
	if err := shortcutConflict(value, h.shortcuts); err != nil {
		return ShortcutState{}, err
	}
	return h.detachShortcuts.Save(value, revision)
}

func (h *HostService) captureShortcut(value string) {
	h.recordMu.Lock()
	window := h.recordingWindow
	h.recordMu.Unlock()
	if window != nil {
		window.DispatchWailsEvent(&application.CustomEvent{Name: "shortcut:recorded", Data: value, Sender: "kernel.shortcuts"})
	}
}
func (h *HostService) startDetachShortcuts() {
	h.detachShortcuts.Start()
	if err := shortcutConflict(h.detachShortcuts.State().Shortcut, h.shortcuts); err != nil {
		h.detachShortcuts.mu.Lock()
		h.detachShortcuts.state.Error = err.Error()
		h.detachShortcuts.mu.Unlock()
		h.detachShortcuts.publish()
	}
}

// System repair is a trusted Shell operation; it is not exposed to UI guests.
func (h *HostService) PreviewSystemRepair(directory string) (kernel.PackagePreview, error) {
	return h.manager.PreviewSystemRepair(directory)
}
func (h *HostService) RepairSystemPlugin(directory, fingerprint, installedFingerprint string) (kernel.PluginInfo, error) {
	h.cancelSearch()
	defer h.cancelSearch()
	result, err := h.manager.RepairSystemPlugin(directory, fingerprint, installedFingerprint)
	if err == nil && h.launcher != nil {
		h.launcher.closePlugin(result.ID)
	}
	return result, err
}

func (h *HostService) PluginData() ([]kernel.PluginDataInfo, error) {
	return h.manager.PluginData()
}
func (h *HostService) ClearPluginData(pluginID, token string) error {
	h.cancelSearch()
	defer h.cancelSearch()
	if err := h.manager.ClearPluginData(pluginID, token); err != nil {
		return err
	}
	if h.launcher != nil {
		h.launcher.closePlugin(pluginID)
	}
	return nil
}

// TrustDevelopmentPlugin belongs only to the bundled management view.
func (h *HostService) TrustDevelopmentPlugin(id, fingerprint string) (kernel.PluginInfo, error) {
	return h.manager.TrustDevelopmentPlugin(id, fingerprint)
}
