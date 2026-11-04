package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// RootLease prevents cooperating hosts from recovering or modifying a live transaction.
// The lock file stays in place; the OS releases its lock when the owner exits.
type RootLease struct {
	file *os.File
	once sync.Once
}

func AcquireRootLease(path string) (*RootLease, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, errors.New("无法创建运行时锁目录")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("无法打开运行时锁")
	}
	if err = lockRoot(file); err != nil {
		file.Close()
		return nil, errors.New("无法锁定插件目录，可能已有宿主使用，请先退出该实例")
	}
	return &RootLease{file: file}, nil
}
func (l *RootLease) Close() { l.once.Do(func() { _ = l.file.Close() }) }
