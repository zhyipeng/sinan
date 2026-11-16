//go:build windows

package main

// newWindowsWndProcInterceptor suppresses the Alt+Space system menu on Sinan's
// own windows. While a recorder is active it additionally feeds the chord to
// the recorder, because Windows consumes Alt+Space before the WebView sees a
// keydown. Otherwise the registered global hot key still delivers WM_HOTKEY,
// which performs the toggle; here we only remove the unwanted menu.
func newWindowsWndProcInterceptor(h *HostService) func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	return func(_ uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
		if !isAltSpaceShortcutMessage(msg, wParam, lParam) {
			return 0, false
		}
		if h.shortcutRecording.Load() {
			// Dispatch off the UI thread: captureShortcut touches the recorder
			// bookkeeping and Wails event bus.
			go h.captureShortcut(altSpaceChord)
			return 0, true
		}
		if h.altSpaceHotkey.Load() {
			return 0, true
		}
		return 0, false
	}
}
