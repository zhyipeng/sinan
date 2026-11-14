//go:build windows

package platform

import (
	"errors"
	"syscall"
	"unsafe"
)

var inputUser32 = syscall.NewLazyDLL("user32.dll")
var inputForeground = inputUser32.NewProc("GetForegroundWindow")

func CaptureInputTarget() uintptr {
	hwnd, _, _ := inputForeground.Call()
	var pid uint32
	inputUser32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == uint32(syscall.Getpid()) {
		return 0
	}
	return hwnd
}
func CheckPasteTarget(target uintptr) error {
	ok, _, _ := inputUser32.NewProc("IsWindow").Call(target)
	if target == 0 || ok == 0 {
		return errors.New("原应用已关闭，请重新唤出插件")
	}
	return nil
}
func ActivateInputTarget(target uintptr) error {
	ok, _, _ := inputUser32.NewProc("SetForegroundWindow").Call(target)
	if ok == 0 {
		return errors.New("系统拒绝恢复原应用焦点")
	}
	return nil
}
func SendPaste(target uintptr) error {
	current, _, _ := inputForeground.Call()
	if current != target {
		return errors.New("原应用未获得焦点，已取消自动粘贴")
	}
	// keybd_event uses the active desktop; validate the foreground immediately first.
	key := inputUser32.NewProc("keybd_event")
	for _, code := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c} {
		state, _, _ := inputUser32.NewProc("GetAsyncKeyState").Call(code)
		if state&0x8000 != 0 {
			return errors.New("请松开修饰键后重试")
		}
	}
	key.Call(0x11, 0, 0, 0)
	key.Call(0x56, 0, 0, 0)
	key.Call(0x56, 0, 2, 0)
	key.Call(0x11, 0, 2, 0)
	return nil
}
