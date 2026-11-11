package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/plugins/system-plugin-manager/scaffold"
)

// These methods are available only to bundled trusted views, never guest pages.
func (h *HostService) ChoosePluginDirectory(ctx context.Context) (string, error) {
	return h.choosePluginDirectory(ctx, "选择插件项目的父目录", true)
}

func (h *HostService) ChooseInstallDirectory(ctx context.Context) (string, error) {
	return h.choosePluginDirectory(ctx, "选择插件安装包目录", false)
}

func (h *HostService) choosePluginDirectory(ctx context.Context, title string, canCreate bool) (string, error) {
	if h.launcher == nil || application.Get() == nil {
		return "", errors.New("当前运行模式不支持目录选择")
	}
	window := h.launcher.caller(ctx)
	if window == nil {
		return "", errors.New("窗口不可用")
	}
	finish := h.launcher.beginDialog(window)
	defer finish()
	return application.Get().Dialog.OpenFile().CanChooseFiles(false).
		CanChooseDirectories(true).CanCreateDirectories(canCreate).
		SetTitle(title).SetButtonText("选择目录").
		AttachToWindow(window).PromptForSingleSelection()
}

func (h *HostService) CreatePluginProject(options scaffold.Options) (scaffold.Result, error) {
	return scaffold.Create(options)
}
