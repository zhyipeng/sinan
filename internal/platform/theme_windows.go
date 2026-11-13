package platform

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

func SystemDark() bool { return w32.IsCurrentlyDarkMode() }

func SetWindowTheme(window unsafe.Pointer, mode string) {
	if window == nil {
		return
	}
	dark := mode == "dark" || mode == "system" && SystemDark()
	w32.SetTheme(uintptr(window), dark)
}
