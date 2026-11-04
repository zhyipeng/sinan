package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"sinan/internal/search"
	protocol "sinan/packages/plugin-protocol"
	"strconv"
	"sync"
)

// HostService is bound only to the trusted Shell, never to third-party UI.
type HostService struct {
	probeEnabled   bool
	manager        *kernel.Manager
	launcher       *launcher
	searchMu       sync.Mutex
	searchSequence uint64
	searchCancel   context.CancelFunc
}

func (h *HostService) Snapshot() kernel.Snapshot { return h.manager.Snapshot() }
func (h *HostService) Refresh() kernel.Snapshot  { return h.manager.Refresh() }
func (h *HostService) Install(directory string, trustNative bool) (kernel.PluginInfo, error) {
	return h.manager.Install(directory, trustNative)
}
func (h *HostService) Execute(pluginID, commandID, input string) (protocol.CommandResult, error) {
	return h.manager.Execute(context.Background(), pluginID, commandID, input)
}
func (h *HostService) Stop(pluginID string) error      { return h.manager.Stop(pluginID) }
func (h *HostService) Uninstall(pluginID string) error { return h.manager.Uninstall(pluginID) }

func (h *HostService) ResizeLauncher(height int) {
	if h.launcher != nil {
		h.launcher.resize(height)
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
func (h *HostService) StartSearch(sequence uint64, query string) {
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
		if h.searchSequence == sequence && ctx.Err() == nil && h.launcher != nil {
			h.launcher.window.DispatchWailsEvent(&application.CustomEvent{Name: "search:update", Data: update, Sender: "kernel.search"})
		}
	})
}

func (h *HostService) CancelSearch(sequence uint64) {
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
