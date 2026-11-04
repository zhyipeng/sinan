//go:build darwin && cgo

package catalog

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

static char *sinanBundleMetadata(const char *path) {
    @autoreleasepool {
        NSBundle *bundle = [NSBundle bundleWithPath:[NSString stringWithUTF8String:path]];
        NSDictionary *info = [NSDictionary dictionaryWithContentsOfFile:[[NSString stringWithUTF8String:path] stringByAppendingPathComponent:@"Contents/Info.plist"]];
        if (!info || ![info[@"CFBundlePackageType"] isEqual:@"APPL"]) return NULL;
        NSArray *languages = [[NSUserDefaults standardUserDefaults] objectForKey:@"AppleLanguages"];
        NSArray *preferred = [NSBundle preferredLocalizationsFromArray:bundle.localizations forPreferences:languages];
        NSString *localization = preferred.firstObject;
        NSString *stringsPath = localization ? [bundle pathForResource:@"InfoPlist" ofType:@"strings" inDirectory:nil forLocalization:localization] : nil;
        NSDictionary *localized = stringsPath ? [NSDictionary dictionaryWithContentsOfFile:stringsPath] : bundle.localizedInfoDictionary;
        // Newer macOS system bundles may ship a loctable instead of lproj strings.
        NSString *tablePath = [bundle pathForResource:@"InfoPlist" ofType:@"loctable"];
        NSDictionary *table = tablePath ? [NSDictionary dictionaryWithContentsOfFile:tablePath] : nil;
        NSMutableArray *available = [NSMutableArray array];
        for (NSString *key in table) {
            if (![key isEqual:@"LocProvenance"] && [table[key] isKindOfClass:[NSDictionary class]]) [available addObject:key];
        }
        NSString *tableLanguage = [NSBundle preferredLocalizationsFromArray:available forPreferences:languages].firstObject;
        if (tableLanguage && [table[tableLanguage] isKindOfClass:[NSDictionary class]]) localized = table[tableLanguage];
        NSMutableDictionary *result = [NSMutableDictionary dictionary];
        for (NSString *key in @[@"CFBundleDisplayName", @"CFBundleName", @"CFBundleIdentifier"]) {
            id value = [key isEqual:@"CFBundleDisplayName"] ? (localized[key] ?: localized[@"CFBundleName"] ?: info[key]) : (localized[key] ?: info[key]);
            if ([value isKindOfClass:[NSString class]]) result[key] = value;
        }
        NSData *data = [NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
        if (!data) return NULL;
        return strdup([[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] autorelease].UTF8String);
    }
}
static char *sinanTransliterate(const char *text) {
    @autoreleasepool {
        NSString *value = [NSString stringWithUTF8String:text];
        value = [value stringByApplyingTransform:NSStringTransformMandarinToLatin reverse:NO];
        value = [value stringByApplyingTransform:NSStringTransformStripCombiningMarks reverse:NO];
        return value ? strdup(value.UTF8String) : NULL;
    }
}
static char *sinanApplicationIcon(const char *path) {
    @autoreleasepool {
        NSImage *image = [[NSWorkspace sharedWorkspace] iconForFile:[NSString stringWithUTF8String:path]];
        CGImageRef source = [image CGImageForProposedRect:NULL context:nil hints:nil];
        if (!source) return NULL;
        CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
        CGContextRef context = CGBitmapContextCreate(NULL, 64, 64, 8, 0, space, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
        CGColorSpaceRelease(space);
        if (!context) return NULL;
        CGContextDrawImage(context, CGRectMake(0,0,64,64), source);
        CGImageRef scaled = CGBitmapContextCreateImage(context);
        NSBitmapImageRep *bitmap = [[[NSBitmapImageRep alloc] initWithCGImage:scaled] autorelease];
        NSData *png = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        CGImageRelease(scaled);
        CGContextRelease(context);
        return png ? strdup([png base64EncodedStringWithOptions:0].UTF8String) : NULL;
    }
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

func readMetadata(path string) (metadata, error) {
	value := C.CString(path)
	defer C.free(unsafe.Pointer(value))
	result := C.sinanBundleMetadata(value)
	if result == nil {
		return metadata{}, errors.New("invalid application bundle")
	}
	defer C.free(unsafe.Pointer(result))
	var meta metadata
	err := json.Unmarshal([]byte(C.GoString(result)), &meta)
	return meta, err
}

func readIcon(path string) string {
	value := C.CString(path)
	defer C.free(unsafe.Pointer(value))
	result := C.sinanApplicationIcon(value)
	if result == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(result))
	return "data:image/png;base64," + C.GoString(result)
}

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
