package platform

import (
	"strconv"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

func CursorScreenID() (string, bool) {
	x, y, ok := w32.GetCursorPos()
	if !ok {
		return "", false
	}
	monitor := w32.MonitorFromPoint(x, y, w32.MONITOR_DEFAULTTONEAREST)
	if monitor == 0 {
		return "", false
	}
	// Match Wails' HMONITOR identity; let its screen manager convert the work
	// area into logical coordinates rather than mixing Win32 pixels with DIP.
	return strconv.Itoa(int(monitor)), true
}

func ScreenWorkArea(string) (WorkArea, bool) { return WorkArea{}, false }
func DetachPanel(unsafe.Pointer)             {}
