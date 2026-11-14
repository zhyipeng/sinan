//go:build linux

package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func inputCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "xdotool", args...).Output()
	return strings.TrimSpace(string(b)), err
}
func CaptureInputTarget() uintptr {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return 0
	}
	value, err := inputCommand("getactivewindow")
	if err != nil {
		return 0
	}
	pid, _ := inputCommand("getwindowpid", value)
	if pid == strconv.Itoa(os.Getpid()) {
		return 0
	}
	id, _ := strconv.ParseUint(value, 10, 64)
	return uintptr(id)
}
func CheckPasteTarget(target uintptr) error {
	if target == 0 || os.Getenv("WAYLAND_DISPLAY") != "" {
		return errors.New("自动粘贴需要 X11 与 xdotool；当前桌面不支持")
	}
	_, err := inputCommand("getwindowpid", strconv.FormatUint(uint64(target), 10))
	if err != nil {
		return errors.New("原应用已关闭或未安装 xdotool")
	}
	return nil
}
func ActivateInputTarget(target uintptr) error {
	_, err := inputCommand("windowactivate", "--sync", strconv.FormatUint(uint64(target), 10))
	if err != nil {
		return errors.New("无法恢复原应用焦点")
	}
	return nil
}
func SendPaste(target uintptr) error {
	current, _ := inputCommand("getactivewindow")
	if current != strconv.FormatUint(uint64(target), 10) {
		return errors.New("原应用未获得焦点，已取消自动粘贴")
	}
	_, err := inputCommand("key", "--clearmodifiers", "ctrl+v")
	if err != nil {
		return errors.New("自动粘贴失败")
	}
	return nil
}
