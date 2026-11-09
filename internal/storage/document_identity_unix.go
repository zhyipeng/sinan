//go:build darwin || linux

package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func documentIdentity(_ *os.File, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("无法识别数据文件")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
