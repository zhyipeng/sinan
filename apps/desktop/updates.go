package main

import (
	"context"
	"errors"
	"sinan/internal/hostupdate"
	settingsupdate "sinan/plugins/system-settings/update"
)

// Host update operations are bound to trusted Shell windows only. They are not
// registered in the third-party plugin Host API or UI bridge.
func (h *HostService) UpdateStatus() settingsupdate.State {
	if h.updates == nil {
		return settingsupdate.State{Phase: "unsupported", Message: "此运行模式不支持自动更新"}
	}
	return h.updates.State()
}
func (h *HostService) CheckHostUpdate(ctx context.Context) (settingsupdate.State, error) {
	if h.updates == nil || h.launcher == nil || h.launcher.caller(ctx) == nil {
		return settingsupdate.State{}, errors.New("窗口不可用")
	}
	deadline, cancel := context.WithCancel(h.updatesContext)
	defer cancel()
	return h.updates.Check(deadline)
}
func (h *HostService) InstallHostUpdate(ctx context.Context) error {
	if h.updates == nil || h.quit == nil || h.launcher == nil || h.launcher.caller(ctx) == nil {
		return errors.New("窗口不可用")
	}
	return h.updates.StartInstall(h.updatesContext, func(plan hostupdate.Plan) error {
		if err := hostupdate.Handoff(plan); err != nil {
			return err
		}
		h.quit()
		return nil
	})
}
