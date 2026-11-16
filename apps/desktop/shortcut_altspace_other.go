//go:build !windows

package main

// The Alt+Space system menu conflict is Windows specific.
func newWindowsWndProcInterceptor(*HostService) func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	return nil
}
