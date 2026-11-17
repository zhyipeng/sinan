package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/paste"
	"sinan/internal/plugin/uiview"
)

type imageBuffer struct{ bytes.Buffer }

func (b *imageBuffer) Write(p []byte) (int, error) {
	if len(p) > paste.MaxImageBytes-b.Len() {
		return 0, errors.New("图片转换后的 PNG 超过 4 MiB")
	}
	return b.Buffer.Write(p)
}

// Decode before attachment; source paths never become resource URLs or page data.
func normalizeImportImage(data []byte) ([]byte, int, int, error) {
	if len(data) == 0 || len(data) > 8<<20 {
		return nil, 0, 0, errors.New("导入图片须小于 8 MiB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > paste.MaxImagePixels/cfg.Height {
		return nil, 0, 0, errors.New("图片无效或超过 1600 万像素")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, errors.New("图片数据损坏")
	}
	var output imageBuffer
	if err = png.Encode(&output, decoded); err != nil {
		return nil, 0, 0, errors.New("图片转换失败或 PNG 超过 4 MiB")
	}
	return output.Bytes(), cfg.Width, cfg.Height, nil
}
func (h *HostService) callOpenImage(ctx context.Context, session *uiview.Session) (json.RawMessage, error) {
	entry, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	defer h.finishWindowCommand(entry, func() {})
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("导入已取消")
	}
	if application.Get() == nil {
		return nil, errors.New("原生选图对话框不可用")
	}
	finish := h.launcher.beginDialog(entry.window)
	defer finish()
	path, err := application.Get().Dialog.OpenFile().CanChooseFiles(true).CanChooseDirectories(false).AddFilter("图片（PNG、JPEG、GIF）", "*.png;*.jpg;*.jpeg;*.gif").SetTitle("导入图片").AttachToWindow(entry.window).PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return json.RawMessage(`null`), nil
	}
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("导入已取消")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("无法打开图片")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, errors.New("请选择小于 8 MiB 的图片文件")
	}
	data, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return nil, errors.New("无法读取图片")
	}
	data, width, height, err := normalizeImportImage(data)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !session.Active() {
		return nil, errors.New("导入已取消")
	}
	resource := session.AttachLaunchImage(data, width, height)
	if resource.URL == "" {
		return nil, errors.New("图片会话已失效")
	}
	return json.Marshal(resource)
}
