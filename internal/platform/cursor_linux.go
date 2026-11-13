//go:build linux && cgo && gtk3

package platform

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <gdk/gdkx.h>

static int sinanPointerMonitor(void) {
    GdkDisplay *display = gdk_display_get_default();
    // Wayland does not provide a global pointer position; do not synthesize it
    // from local coordinates or treat the compositor's fallback as a real hit.
    if (!display || !GDK_IS_X11_DISPLAY(display)) return -1;
    GdkSeat *seat = gdk_display_get_default_seat(display);
    GdkDevice *pointer = seat ? gdk_seat_get_pointer(seat) : NULL;
    if (!pointer) return -1;
    gint x, y;
    gdk_device_get_position(pointer, NULL, &x, &y);
    GdkMonitor *monitor = gdk_display_get_monitor_at_point(display, x, y);
    for (int i = 0; i < gdk_display_get_n_monitors(display); i++) {
        if (gdk_display_get_monitor(display, i) == monitor) return i;
    }
    return -1;
}
static int sinanMonitorWorkArea(int index, GdkRectangle *area) {
    GdkDisplay *display = gdk_display_get_default();
    if (!display || !GDK_IS_X11_DISPLAY(display) || index < 0 || index >= gdk_display_get_n_monitors(display)) return 0;
    gdk_monitor_get_workarea(gdk_display_get_monitor(display, index), area);
    return area->width > 0 && area->height > 0;
}
*/
import "C"

import (
	"strconv"
	"unsafe"
)

// Caller must run GDK queries on the GTK UI thread.
func CursorScreenID() (string, bool) {
	index := int(C.sinanPointerMonitor())
	if index < 0 {
		return "", false
	}
	return strconv.Itoa(index), true
}

func ScreenWorkArea(id string) (WorkArea, bool) {
	index, err := strconv.Atoi(id)
	if err != nil || index < 0 || index > 1024 {
		return WorkArea{}, false
	}
	var area C.GdkRectangle
	if C.sinanMonitorWorkArea(C.int(index), &area) == 0 {
		return WorkArea{}, false
	}
	return WorkArea{X: int(area.x), Y: int(area.y), Width: int(area.width), Height: int(area.height)}, true
}

func DetachPanel(unsafe.Pointer) {}
