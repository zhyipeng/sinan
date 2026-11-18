//go:build !windows || (!amd64 && !arm64) || native_webview2loader

package uihost

import (
	"time"
	"unsafe"
)

const EmbeddedLoadTimeout = 3 * time.Second

func WarmEmbedded(func() unsafe.Pointer, func(func())) {}
func CloseWarmEmbedded(func(func()))                   {}
