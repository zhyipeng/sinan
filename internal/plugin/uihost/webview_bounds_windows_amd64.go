//go:build windows && !native_webview2loader

package uihost

import "unsafe"

// Windows x64 passes a 16-byte value aggregate indirectly.
func setWebviewBounds(controller *webviewCOM, rect *webviewRect) error {
	return controller.call(6, uintptr(unsafe.Pointer(rect)))
}
