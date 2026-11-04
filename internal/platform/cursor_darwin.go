//go:build darwin && !ios && cgo

package platform

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

static CGDirectDisplayID sinanCursorDisplay(void) {
    CGEventRef event = CGEventCreate(NULL);
    if (event == NULL) return kCGNullDirectDisplay;
    CGPoint point = CGEventGetLocation(event);
    CFRelease(event);

    CGDirectDisplayID display = kCGNullDirectDisplay;
    uint32_t count = 0;
    if (CGGetDisplaysWithPoint(point, 1, &display, &count) != kCGErrorSuccess || count == 0) {
        return kCGNullDirectDisplay;
    }
    return display;
}
*/
import "C"

import "strconv"

// CursorScreenID returns the display containing the pointer, using the same
// CGDirectDisplayID identity as Wails. This avoids mixing Cocoa, Quartz and DIP
// coordinates when monitors have different scales or negative desktop origins.
func CursorScreenID() (string, bool) {
	id := C.sinanCursorDisplay()
	if id == C.kCGNullDirectDisplay {
		return "", false
	}
	return strconv.FormatUint(uint64(id), 10), true
}
