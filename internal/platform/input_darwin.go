//go:build darwin && !ios && cgo

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework ApplicationServices
#import <Cocoa/Cocoa.h>
#import <ApplicationServices/ApplicationServices.h>
#include <unistd.h>
static long sinanInputTarget(void) {
 @autoreleasepool { pid_t pid = NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier; return pid == getpid() ? 0 : pid; }
}
static bool sinanCanPaste(long pid) { return pid > 0 && [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid] != nil; }
static bool sinanAccessibilityTrusted(void) { return AXIsProcessTrusted(); }
// Runs only on the AppKit main thread, after an explicit paste request.
static void sinanAccessibilityGuide(void) {
 @autoreleasepool {
  if (AXIsProcessTrusted()) return;
  NSAlert *alert = [[[NSAlert alloc] init] autorelease];
  alert.alertStyle = NSAlertStyleInformational;
  alert.messageText = @"允许司南自动粘贴";
  alert.informativeText = @"请在“隐私与安全性 → 辅助功能”中开启司南，返回后重新选择条目粘贴。内容已复制，也可以手动粘贴。";
  [alert addButtonWithTitle:@"打开系统设置"];
  [alert addButtonWithTitle:@"稍后"];
  [NSApp activateIgnoringOtherApps:YES];
  if ([alert runModal] == NSAlertFirstButtonReturn) {
   NSURL *url = [NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"];
   if (![NSWorkspace.sharedWorkspace openURL:url]) {
    NSAlert *failure = [[[NSAlert alloc] init] autorelease];
    failure.messageText = @"无法打开系统设置";
    failure.informativeText = @"请手动打开“系统设置 → 隐私与安全性 → 辅助功能”，并开启司南。";
    [failure addButtonWithTitle:@"知道了"];
    [failure runModal];
   }
  }
 }
}
static bool sinanActivateInput(long pid) {
 @autoreleasepool { NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid]; return app && [app activateWithOptions:0]; }
}
static bool sinanSendPaste(long pid) {
 @autoreleasepool {
  if (NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier != pid || !AXIsProcessTrusted()) return false;
  if (CGEventSourceFlagsState(kCGEventSourceStateCombinedSessionState) & (kCGEventFlagMaskCommand | kCGEventFlagMaskControl | kCGEventFlagMaskShift | kCGEventFlagMaskAlternate)) return false;
  CGEventRef down = CGEventCreateKeyboardEvent(NULL, 9, true), up = CGEventCreateKeyboardEvent(NULL, 9, false);
  if (!down || !up) { if(down)CFRelease(down); if(up)CFRelease(up); return false; }
  CGEventSetFlags(down, kCGEventFlagMaskCommand); CGEventSetFlags(up, kCGEventFlagMaskCommand);
  CGEventPost(kCGHIDEventTap, down); CGEventPost(kCGHIDEventTap, up); CFRelease(down); CFRelease(up); return true;
 }
}
*/
import "C"
import "errors"

func CaptureInputTarget() uintptr { return uintptr(C.sinanInputTarget()) }
func CheckPasteTarget(target uintptr) error {
	if !bool(C.sinanAccessibilityTrusted()) {
		return ErrAccessibilityPermission
	}
	if !bool(C.sinanCanPaste(C.long(target))) {
		return errors.New("原应用已关闭或不可用，请重新唤出插件")
	}
	return nil
}
func ActivateInputTarget(target uintptr) error {
	if !bool(C.sinanActivateInput(C.long(target))) {
		return errors.New("无法恢复原应用")
	}
	return nil
}
func SendPaste(target uintptr) error {
	if !bool(C.sinanSendPaste(C.long(target))) {
		return errors.New("原应用未获得焦点，已取消自动粘贴")
	}
	return nil
}

// ShowAccessibilityPermissionGuide must run on the AppKit main thread.
func ShowAccessibilityPermissionGuide() { C.sinanAccessibilityGuide() }
