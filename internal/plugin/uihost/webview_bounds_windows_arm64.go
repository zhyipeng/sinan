//go:build windows && !native_webview2loader

package uihost

// Windows ARM64 passes RECT by value in two registers, unlike Windows x64.
// Verified with clang --target=aarch64-pc-windows-msvc against a C signature.
func setWebviewBounds(controller *webviewCOM, rect *webviewRect) error {
	lo := uintptr(uint32(rect.Left)) | uintptr(uint32(rect.Top))<<32
	hi := uintptr(uint32(rect.Right)) | uintptr(uint32(rect.Bottom))<<32
	return controller.call(6, lo, hi)
}
