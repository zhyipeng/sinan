//go:build linux && cgo && gtk3

package platform

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
static int sinanSetWindowOpacity(void *pointer, int percent) {
    GtkWidget *window = GTK_WIDGET(pointer);
    GdkScreen *screen = gtk_widget_get_screen(window);
    if (!screen || !gdk_screen_is_composited(screen)) return 0;
    gtk_widget_set_opacity(window, percent / 100.0);
    return 1;
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

func SetWindowOpacity(window unsafe.Pointer, percent int) error {
	if err := validateWindowOpacity(percent); err != nil {
		return err
	}
	if window == nil {
		return errors.New("窗口尚未就绪")
	}
	if C.sinanSetWindowOpacity(window, C.int(percent)) == 0 {
		return errors.New("当前 Linux 桌面未提供窗口合成，无法调整透明度")
	}
	return nil
}
