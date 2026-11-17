package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

// Visibility controls apply only to the live calling view, never a supplied ID.
func (h *HostService) callWindowVisibility(ctx context.Context, session *uiview.Session, req protocol.UIRequest) (json.RawMessage, error) {
	entry, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	h.finishWindowCommand(entry, func() {
		if req.Method == "window.hide" {
			h.hideCommandWindows(entry.window)
		} else {
			entry.view.Show()
			entry.window.Show()
			entry.window.Focus()
		}
	})
	return json.RawMessage(`null`), nil
}

// The exporter receives <opaque input>:<offset> and returns bounded base64
// chunks in CommandResult.Text. No chosen path is ever disclosed to the plugin.
func collectPNG(ctx context.Context, input string, execute func(context.Context, string) (json.RawMessage, error)) ([]byte, error) {
	if len(input) != 32 || strings.Contains(input, ":") {
		return nil, errors.New("PNG 导出标识无效")
	}
	var data []byte
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := execute(ctx, fmt.Sprintf("%s:%d", input, len(data)))
		if err != nil {
			return nil, err
		}
		if len(raw) > uiview.MaxCallResultBytes {
			return nil, errors.New("PNG 分块响应过大")
		}
		var result protocol.CommandResult
		var chunk struct {
			Size  int    `json:"size"`
			Data  string `json:"data"`
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &result) != nil || json.Unmarshal([]byte(result.Text), &chunk) != nil || chunk.Error != "" {
			return nil, errors.New("PNG 导出失败")
		}
		if chunk.Size < 1 || chunk.Size > 8<<20 || (total != 0 && total != chunk.Size) {
			return nil, errors.New("PNG 大小无效")
		}
		total = chunk.Size
		block, err := base64.StdEncoding.Strict().DecodeString(chunk.Data)
		if err != nil || len(block) == 0 || len(block) > 36<<10 || len(data)+len(block) > total {
			return nil, errors.New("PNG 分块无效")
		}
		data = append(data, block...)
		if len(data) == total {
			break
		}
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil, errors.New("PNG 图片无效或尺寸过大")
	}
	if _, err = png.Decode(bytes.NewReader(data)); err != nil {
		return nil, errors.New("PNG 图片损坏")
	}
	return data, nil
}
func (h *HostService) callSavePNG(ctx context.Context, session *uiview.Session, req protocol.UIRequest) (json.RawMessage, error) {
	entry, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	defer h.finishWindowCommand(entry, func() {})
	data, err := collectPNG(ctx, req.Input, func(ctx context.Context, input string) (json.RawMessage, error) {
		return h.manager.ExecuteViewCommand(ctx, session, req.CommandID, input)
	})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("保存已取消")
	}
	if application.Get() == nil {
		return nil, errors.New("原生保存对话框不可用")
	}
	finish := h.launcher.beginDialog(entry.window)
	defer finish()
	path, err := application.Get().Dialog.SaveFile().SetFilename("截图-"+time.Now().Format("20060102-150405")+".png").AddFilter("PNG 图片", "*.png").AllowsOtherFileTypes(false).CanCreateDirectories(true).AttachToWindow(entry.window).PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return json.RawMessage(`{"saved":false}`), nil
	}
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("保存已取消")
	}
	// Never append a different suffix after confirmation: that would overwrite a
	// path the native dialog did not confirm.
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		return nil, errors.New("请选择 .png 文件名")
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return nil, errors.New("无法保存 PNG 文件")
	}
	return json.RawMessage(`{"saved":true}`), nil
}
