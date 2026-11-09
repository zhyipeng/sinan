//go:build !darwin && !linux && !windows

package storage

import (
	"errors"
	"os"
)

func documentIdentity(*os.File, os.FileInfo) (string, error) {
	return "", errors.New("此平台暂不支持数据文件身份检查")
}
