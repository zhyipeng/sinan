package hostupdate

import (
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}
func WaitParent(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return errors.New("无法等待旧进程退出")
	}
	defer windows.CloseHandle(handle)
	result, err := windows.WaitForSingleObject(handle, 30000)
	if err != nil || result != windows.WAIT_OBJECT_0 {
		return errors.New("旧进程未退出，已取消更新")
	}
	return nil
}
