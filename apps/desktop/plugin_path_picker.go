package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/plugin/uiview"
)

func (h *HostService) callPickPath(ctx context.Context, session *uiview.Session, kind string) (json.RawMessage, error) {
	entry, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	defer h.finishWindowCommand(entry, func() {})
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("路径选择已取消")
	}
	app := application.Get()
	if app == nil {
		return nil, errors.New("原生路径选择不可用")
	}
	finish := h.launcher.beginDialog(entry.window)
	defer finish()

	directory := kind == "directory"
	title := "选择文件"
	if directory {
		title = "选择目录"
	}
	path, err := app.Dialog.OpenFile().CanChooseFiles(!directory).
		CanChooseDirectories(directory).CanCreateDirectories(false).
		ShowHiddenFiles(true).SetTitle(title).
		AttachToWindow(entry.window).PromptForSingleSelection()
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("路径选择已取消")
	}
	if err != nil {
		// Wails returns its internal cfd cancellation sentinel on Windows.
		if runtime.GOOS == "windows" && err.Error() == "cancelled by user" {
			return json.RawMessage("null"), nil
		}
		return nil, errors.New("原生路径选择失败")
	}
	if path == "" {
		return json.RawMessage("null"), nil
	}
	if !filepath.IsAbs(path) || len(path) > 16<<10 {
		return nil, errors.New("所选路径无效")
	}
	return json.Marshal(path)
}
