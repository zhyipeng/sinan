package platform

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"unsafe"
)

func SetWindowOpacity(window unsafe.Pointer, percent int) error {
	if err := validateWindowOpacity(percent); err != nil {
		return err
	}
	if window == nil {
		return errors.New("窗口尚未就绪")
	}
	hwnd := w32.HWND(uintptr(window))
	style := w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)
	if style&w32.WS_EX_LAYERED == 0 {
		if percent == 100 {
			return nil
		}
		w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, uint32(style)|w32.WS_EX_LAYERED)
		if w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)&w32.WS_EX_LAYERED == 0 {
			return errors.New("当前窗口无法启用透明度")
		}
	}
	if !w32.SetLayeredWindowAttributes(hwnd, 0, byte((percent*255+50)/100), w32.LWA_ALPHA) {
		if style&w32.WS_EX_LAYERED == 0 {
			w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, uint32(style))
		}
		return errors.New("无法调整窗口透明度")
	}
	return nil
}
