//go:build darwin || linux

package hostupdate

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func WaitParent(pid int) error {
	for i := 0; i < 300; i++ {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return errors.New("无法等待旧进程退出")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("旧进程未退出，已取消更新")
}
