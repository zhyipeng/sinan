//go:build (!darwin && !linux) || ios || (darwin && !cgo) || (linux && (!cgo || !gtk3))

package uihost

import (
	"context"
	"errors"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
	"unsafe"
)

const EmbeddedSupported = false

type Embedded struct{}

func PrepareEmbedded(context.Context, *uiview.Session, func() unsafe.Pointer, protocol.ThemeState, string, func(int), func(func())) (*Embedded, error) {
	return nil, errors.New("当前平台尚不支持主窗口插件界面")
}
func (*Embedded) Show()                        {}
func (*Embedded) Close()                       {}
func (*Embedded) SetTheme(protocol.ThemeState) {}
func (*Embedded) Done() <-chan struct{}        { done := make(chan struct{}); close(done); return done }

func (*Embedded) SetDetachShortcut(string, uint64) {}

func (*Embedded) SetDetached() {}
