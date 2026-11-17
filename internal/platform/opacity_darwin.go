//go:build darwin && !ios && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static void sinanSetWindowOpacity(void *pointer, int percent) {
    [(NSWindow *)pointer setAlphaValue:percent / 100.0];
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

// SetWindowOpacity must be called on the native UI thread.
func SetWindowOpacity(window unsafe.Pointer, percent int) error {
	if err := validateWindowOpacity(percent); err != nil {
		return err
	}
	if window == nil {
		return errors.New("窗口尚未就绪")
	}
	C.sinanSetWindowOpacity(window, C.int(percent))
	return nil
}
