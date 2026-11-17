// Package startup registers the current executable for user login without
// starting it or touching another user's configuration.
package startup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

type Registration interface {
	Enabled() (bool, error)
	SetEnabled(bool) error
}

func New(executable string) (Registration, error) {
	if !filepath.IsAbs(executable) || bytes.ContainsAny([]byte(executable), "\x00\r\n") {
		return nil, errors.New("无法确定开机自启程序")
	}
	return newRegistration(executable)
}

type fileRegistration struct {
	path    string
	content []byte
}

func (r fileRegistration) Enabled() (bool, error) {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("无法读取开机自启配置")
	}
	return bytes.Equal(data, r.content), nil
}

func (r fileRegistration) SetEnabled(enabled bool) error {
	if !enabled {
		if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
			return errors.New("无法移除开机自启配置")
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return errors.New("无法创建开机自启目录")
	}
	f, err := os.CreateTemp(filepath.Dir(r.path), ".sinan-startup-*")
	if err != nil {
		return errors.New("无法写入开机自启配置")
	}
	defer os.Remove(f.Name())
	_, err = f.Write(r.content)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("无法写入开机自启配置")
	}
	if os.Rename(f.Name(), r.path) != nil {
		return errors.New("无法保存开机自启配置")
	}
	return nil
}
