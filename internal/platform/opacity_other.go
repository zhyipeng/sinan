//go:build (!darwin && !windows && !linux) || ios || (darwin && !cgo) || (linux && (!cgo || !gtk3))

package platform

import (
	"errors"
	"unsafe"
)

func SetWindowOpacity(unsafe.Pointer, int) error {
	return errors.New("当前构建不支持窗口透明度")
}
