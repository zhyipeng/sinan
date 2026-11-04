package main

import (
	"context"
	"sinan/internal/kernel"
	protocol "sinan/packages/plugin-protocol"
)

// HostService is bound only to the trusted Shell, never to third-party UI.
type HostService struct{ manager *kernel.Manager }

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
