package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sinan/internal/kernel"
	"sinan/plugins/system-plugin-store/catalog"
)

func (h *HostService) pluginStore() (*catalog.Service, error) {
	h.storeMu.Lock()
	defer h.storeMu.Unlock()
	if h.store == nil {
		root, err := os.UserCacheDir()
		if err != nil {
			return nil, errors.New("无法确定商店缓存目录")
		}
		h.store = catalog.NewService(h.manager, filepath.Join(root, "Sinan", "plugin-store"))
	}
	return h.store, nil
}
func (h *HostService) StoreCatalog(ctx context.Context, refresh bool) (catalog.View, error) {
	service, err := h.pluginStore()
	if err != nil {
		return catalog.View{}, err
	}
	return service.Load(ctx, refresh)
}
func (h *HostService) StorePreview(ctx context.Context, id, version, revision string) (catalog.Preview, error) {
	service, err := h.pluginStore()
	if err != nil {
		return catalog.Preview{}, err
	}
	return service.Prepare(ctx, id, version, revision)
}
func (h *HostService) StoreApply(ctx context.Context, ticket string, trustNative bool) (kernel.PluginInfo, error) {
	service, err := h.pluginStore()
	if err != nil {
		return kernel.PluginInfo{}, err
	}
	h.cancelSearch()
	defer h.cancelSearch()
	result, err := service.Apply(ctx, ticket, trustNative)
	if err == nil && h.launcher != nil {
		h.launcher.closePlugin(result.ID)
	}
	return result, err
}
func (h *HostService) StoreCancel(ticket string) {
	h.storeMu.Lock()
	service := h.store
	h.storeMu.Unlock()
	if service != nil {
		service.Cancel(ticket)
	}
}
func (h *HostService) closePluginStore() {
	h.storeMu.Lock()
	service := h.store
	h.storeMu.Unlock()
	if service != nil {
		service.Close()
	}
}
