//go:build darwin && cgo

package searchtext

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>
static char *sinanTransliterate(const char *text) {
    @autoreleasepool {
        NSString *value = [NSString stringWithUTF8String:text];
        value = [value stringByApplyingTransform:NSStringTransformMandarinToLatin reverse:NO];
        value = [value stringByApplyingTransform:NSStringTransformStripCombiningMarks reverse:NO];
        return value ? strdup(value.UTF8String) : NULL;
    }
}
*/
import "C"

import "unsafe"

func transliterate(text string) string {
	value := C.CString(text)
	defer C.free(unsafe.Pointer(value))
	result := C.sinanTransliterate(value)
	if result == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result)
}
