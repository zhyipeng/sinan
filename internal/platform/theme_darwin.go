//go:build darwin && !ios && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
static bool sinanSystemDark(void) {
 @autoreleasepool { return [[[NSUserDefaults standardUserDefaults] stringForKey:@"AppleInterfaceStyle"] isEqualToString:@"Dark"]; }
}
static void sinanWindowTheme(void *pointer,int mode) {
 NSWindow *window=(NSWindow *)pointer;
 window.appearance=mode==2?[NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]:mode==1?[NSAppearance appearanceNamed:NSAppearanceNameAqua]:nil;
}
*/
import "C"
import "unsafe"

func SystemDark() bool { return bool(C.sinanSystemDark()) }
func SetWindowTheme(window unsafe.Pointer, mode string) {
	value := 0
	if mode == "light" {
		value = 1
	}
	if mode == "dark" {
		value = 2
	}
	C.sinanWindowTheme(window, C.int(value))
}
