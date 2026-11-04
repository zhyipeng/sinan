//go:build darwin && !ios && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework Cocoa
#include <CoreGraphics/CoreGraphics.h>
#import <Cocoa/Cocoa.h>

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

typedef struct {
    double x, y, width, height;
    bool found;
} SinanWorkArea;

static SinanWorkArea sinanScreenWorkArea(CGDirectDisplayID displayID) {
    @autoreleasepool {
        SinanWorkArea result = {0};
        NSArray<NSScreen *> *screens = [NSScreen screens];
        NSScreen *primary = [screens firstObject];
        if (primary == nil) return result;
        CGFloat primaryHeight = primary.frame.size.height;
        for (NSScreen *screen in screens) {
            NSNumber *number = screen.deviceDescription[@"NSScreenNumber"];
            if (number.unsignedIntValue != displayID) continue;
            NSRect frame = screen.visibleFrame;
            result.x = frame.origin.x;
            result.y = primaryHeight - NSMaxY(frame);
            result.width = frame.size.width;
            result.height = frame.size.height;
            result.found = true;
            break;
        }
        return result;
    }
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

// ScreenWorkArea reads the native logical work area without Wails' mixed-DPI
// screen rearrangement. The caller must execute this AppKit query on the main thread.
func ScreenWorkArea(id string) (WorkArea, bool) {
	number, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return WorkArea{}, false
	}
	area := C.sinanScreenWorkArea(C.CGDirectDisplayID(number))
	return WorkArea{X: int(area.x), Y: int(area.y), Width: int(area.width), Height: int(area.height)}, bool(area.found)
}
