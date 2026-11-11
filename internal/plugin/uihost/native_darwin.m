//go:build darwin && !ios && cgo

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import "native_darwin.h"
#include <stdatomic.h>

struct SinanUIControl { atomic_bool revoked; };
SinanUIControl *sinanUIControlNew(void) {
    SinanUIControl *control = calloc(1, sizeof(SinanUIControl));
    if (control) atomic_init(&control->revoked, false);
    return control;
}
void sinanUIControlRevoke(SinanUIControl *control) { atomic_store(&control->revoked, true); }
void sinanUIControlFree(SinanUIControl *control) { free(control); }

static BOOL matchesDetachKey(unsigned short keyCode, NSEventModifierFlags modifiers, NSString *shortcut) {
    NSArray<NSString *> *parts = [shortcut componentsSeparatedByString:@"+"];
    NSDictionary *codes = @{@"A":@0, @"S":@1, @"D":@2, @"F":@3, @"H":@4, @"G":@5, @"Z":@6, @"X":@7, @"C":@8, @"V":@9, @"B":@11, @"Q":@12, @"W":@13, @"E":@14, @"R":@15, @"Y":@16, @"T":@17, @"1":@18, @"2":@19, @"3":@20, @"4":@21, @"6":@22, @"5":@23, @"9":@25, @"7":@26, @"8":@28, @"0":@29, @"O":@31, @"U":@32, @"I":@34, @"P":@35, @"Enter":@36, @"L":@37, @"J":@38, @"K":@40, @"N":@45, @"M":@46, @"Comma":@43, @"Tab":@48, @"Space":@49, @"Backspace":@51, @"Delete":@117, @"Home":@115, @"End":@119, @"Page Up":@116, @"Page Down":@121, @"Left":@123, @"Right":@124, @"Down":@125, @"Up":@126, @"F1":@122, @"F2":@120, @"F3":@99, @"F4":@118, @"F5":@96, @"F6":@97, @"F7":@98, @"F8":@100, @"F9":@101, @"F10":@109, @"F11":@103, @"F12":@111, @"F13":@105, @"F14":@107, @"F15":@113, @"F16":@106, @"F17":@64, @"F18":@79, @"F19":@80, @"F20":@90};
    NSNumber *code = codes[parts.lastObject];
    if (!code || keyCode != code.unsignedShortValue) return NO;
    NSEventModifierFlags expected = 0;
    for (NSUInteger i=0; i+1<parts.count; i++) {
        NSString *part = parts[i];
        if ([part isEqual:@"Cmd"]) expected |= NSEventModifierFlagCommand;
        else if ([part isEqual:@"Ctrl"]) expected |= NSEventModifierFlagControl;
        else if ([part isEqual:@"Alt"]) expected |= NSEventModifierFlagOption;
        else if ([part isEqual:@"Shift"]) expected |= NSEventModifierFlagShift;
        else return NO;
    }
    NSEventModifierFlags mask = NSEventModifierFlagCommand | NSEventModifierFlagShift | NSEventModifierFlagOption | NSEventModifierFlagControl;
    return expected != 0 && (modifiers & mask) == expected;
}
int sinanUIDetachKeyMatches(char *shortcut, unsigned short keyCode, unsigned long modifiers) {
    @autoreleasepool { return matchesDetachKey(keyCode, modifiers, [NSString stringWithUTF8String:shortcut]); }
}
static BOOL matchesDetachShortcut(NSEvent *event, NSString *shortcut, NSWindow *window) {
    if (event.type != NSEventTypeKeyDown || event.isARepeat) return NO;
    id responder = window.firstResponder;
    if ([responder respondsToSelector:@selector(hasMarkedText)] && [(id<NSTextInputClient>)responder hasMarkedText]) return NO;
    return matchesDetachKey(event.keyCode, event.modifierFlags, shortcut);
}
static NSString *detachTooltip(NSString *shortcut) {
    NSDictionary *symbols = @{@"Cmd":@"⌘", @"Ctrl":@"⌃", @"Alt":@"⌥", @"Shift":@"⇧"};
    NSMutableString *label = [NSMutableString stringWithString:@"独立窗口（"];
    for (NSString *part in [shortcut componentsSeparatedByString:@"+"])
        [label appendString:symbols[part] ?: part];
    [label appendString:@"）"];
    return label;
}

// Match the trusted Shell's .view-header without loading plugin HTML into it.
static const CGFloat SinanHeaderHeight = 58;

static NSColor *headerToken(NSDictionary *theme, NSString *token, NSAppearance *appearance) {
    BOOL dark = [[appearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]] isEqual:NSAppearanceNameDarkAqua];
    NSString *mode = theme[@"mode"];
    if ([mode isEqual:@"dark"]) dark = YES;
    else if ([mode isEqual:@"light"]) dark = NO;
    NSString *hex = theme[dark ? @"dark" : @"light"][@"tokens"][token];
    unsigned int rgb = 0;
    if ([hex isKindOfClass:NSString.class] && hex.length == 7 && [hex hasPrefix:@"#"] &&
        [[NSScanner scannerWithString:[hex substringFromIndex:1]] scanHexInt:&rgb])
        return [NSColor colorWithSRGBRed:((rgb >> 16) & 255)/255.0 green:((rgb >> 8) & 255)/255.0 blue:(rgb & 255)/255.0 alpha:1];
    return NSColor.labelColor;
}

@interface SinanHeaderButton : NSButton
@property BOOL hovered;
@property NSTrackingArea *hoverTracking;
@property NSColor *hoverColor;
@property NSColor *focusColor;
@end
@implementation SinanHeaderButton
- (BOOL)acceptsFirstResponder { return YES; }
- (void)updateTrackingAreas {
    [super updateTrackingAreas];
    if (self.hoverTracking) [self removeTrackingArea:self.hoverTracking];
    self.hoverTracking = [[NSTrackingArea alloc] initWithRect:NSZeroRect options:NSTrackingMouseEnteredAndExited | NSTrackingActiveInKeyWindow | NSTrackingInVisibleRect owner:self userInfo:nil];
    [self addTrackingArea:self.hoverTracking];
}
- (void)mouseEntered:(NSEvent *)event { self.hovered = YES; self.needsDisplay = YES; }
- (void)mouseExited:(NSEvent *)event { self.hovered = NO; self.needsDisplay = YES; }
- (void)drawRect:(NSRect)dirtyRect {
    if (self.hovered || [self.cell isHighlighted]) {
        [self.hoverColor setFill];
        [[NSBezierPath bezierPathWithRoundedRect:self.bounds xRadius:7 yRadius:7] fill];
    }
    [super drawRect:dirtyRect];
    if (self.window.isKeyWindow && self.window.firstResponder == self) {
        [self.focusColor setStroke];
        NSBezierPath *ring = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(self.bounds,1,1) xRadius:6 yRadius:6];
        ring.lineWidth = 2;
        [ring stroke];
    }
}
@end

@interface SinanPanelHeader : NSView
@property NSDictionary *theme;
@property SinanHeaderButton *backButton;
@property SinanHeaderButton *detachButton;
@property NSTextField *titleLabel;
@property NSTextField *sourceLabel;
@property BOOL detached;
- (instancetype)initWithTitle:(NSString *)title source:(NSString *)source target:(id)target;
- (void)refreshTheme;
@end
@implementation SinanPanelHeader
- (instancetype)initWithTitle:(NSString *)title source:(NSString *)source target:(id)target {
    if (!(self = [super initWithFrame:NSZeroRect])) return nil;
    self.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    self.backButton = [self button:@"←" label:@"返回搜索" target:target action:@selector(backPanel:)];
    self.detachButton = [self button:@"↗" label:@"独立窗口" target:target action:@selector(detachPanel:)];
    self.titleLabel = [NSTextField labelWithString:title ?: @"司南插件"];
    self.titleLabel.font = [NSFont systemFontOfSize:15 weight:NSFontWeightMedium];
    self.titleLabel.lineBreakMode = NSLineBreakByTruncatingTail;
    self.titleLabel.cell.wraps = NO;
    self.sourceLabel = [NSTextField labelWithString:source ?: @""];
    self.sourceLabel.font = [NSFont systemFontOfSize:11];
    self.sourceLabel.lineBreakMode = NSLineBreakByTruncatingTail;
    self.sourceLabel.cell.wraps = NO;
    [self addSubview:self.titleLabel];
    [self addSubview:self.sourceLabel];
    [self refreshTheme];
    return self;
}
- (SinanHeaderButton *)button:(NSString *)symbol label:(NSString *)label target:(id)target action:(SEL)action {
    SinanHeaderButton *button = [[SinanHeaderButton alloc] initWithFrame:NSZeroRect];
    button.title = symbol;
    button.target = target;
    button.action = action;
    [button setButtonType:NSButtonTypeMomentaryPushIn];
    button.bordered = NO;
    button.font = [NSFont systemFontOfSize:18];
    button.focusRingType = NSFocusRingTypeNone;
    [(NSButtonCell *)button.cell setHighlightsBy:NSNoCellMask];
    [button setAccessibilityLabel:label];
    [self addSubview:button];
    return button;
}
- (NSColor *)color:(NSString *)token {
    __weak SinanPanelHeader *weakSelf = self;
    return [NSColor colorWithName:nil dynamicProvider:^NSColor *(NSAppearance *appearance) {
        return headerToken(weakSelf.theme,token,appearance);
    }];
}
- (void)refreshTheme {
    if (!self.backButton || !self.detachButton) return;
    self.titleLabel.textColor = [self color:@"text"];
    self.sourceLabel.textColor = [self color:@"muted"];
    for (SinanHeaderButton *button in @[self.backButton,self.detachButton]) {
        button.attributedTitle = [[NSAttributedString alloc] initWithString:button.title attributes:@{NSFontAttributeName:[NSFont systemFontOfSize:18],NSForegroundColorAttributeName:[self color:@"muted"]}];
        button.hoverColor = [self color:@"hover"];
        button.focusColor = [self color:@"accent"];
        button.needsDisplay = YES;
    }
    self.needsDisplay = YES;
}
- (void)viewDidChangeEffectiveAppearance {
    [super viewDidChangeEffectiveAppearance];
    [self refreshTheme];
}
- (void)layout {
    [super layout];
    CGFloat width = self.bounds.size.width;
    self.backButton.frame = NSMakeRect(16,11,42,36);
    self.detachButton.frame = NSMakeRect(MAX(70,width-58),11,42,36);
    CGFloat sourceWidth = self.sourceLabel.stringValue.length ? MIN(180,[self.sourceLabel.stringValue sizeWithAttributes:@{NSFontAttributeName:self.sourceLabel.font}].width+2) : 0;
    CGFloat trailing = self.detached ? 16 : 70;
    CGFloat titleWidth = MAX(0,width-70-trailing-(sourceWidth ? sourceWidth+12 : 0));
    self.titleLabel.frame = NSMakeRect(70,19,titleWidth,20);
    self.sourceLabel.frame = NSMakeRect(70+titleWidth+12,21,sourceWidth,16);
}
- (void)drawRect:(NSRect)dirtyRect {
    [[self color:@"background"] setFill]; NSRectFill(self.bounds);
    [[self color:@"line"] setFill]; NSRectFill(NSMakeRect(0,0,self.bounds.size.width,1));
}
@end

@interface SinanMainPanel : NSPanel
@end
@implementation SinanMainPanel
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return NO; }
@end

static void positionPanel(NSWindow *window) {
    NSScreen *target = NSScreen.mainScreen;
    for (NSScreen *screen in NSScreen.screens)
        if (NSMouseInRect(NSEvent.mouseLocation, screen.frame, NO)) { target = screen; break; }
    NSRect work = target.visibleFrame;
    NSRect frame = window.frame;
    frame.size.width = MIN(680, work.size.width);
    frame.size.height = MIN(530, work.size.height * 0.8 - 16);
    frame.origin.x = NSMidX(work) - frame.size.width / 2;
    frame.origin.y = NSMaxY(work) - work.size.height * 0.2 - frame.size.height;
    [window setFrame:frame display:YES];
}

@interface SinanUIProbe : NSObject <WKNavigationDelegate, WKUIDelegate, WKURLSchemeHandler, WKScriptMessageHandler, WKScriptMessageHandlerWithReply, NSWindowDelegate>
@property unsigned long long handle;
@property SinanUIControl *control;
@property BOOL ownsControl;
@property NSTimer *embeddedTimer;
@property id embeddedKeyMonitor;
@property id embeddedResizeObserver;
@property NSString *entry;
@property NSString *csp;
@property NSString *target;
@property WKWebView *view;
@property BOOL probe;
@property BOOL attached;
@property NSString *detachShortcut;
@property NSButton *detachButton;
@property SinanPanelHeader *header;
@property BOOL detachRequested;
@property BOOL backRequested;
@property BOOL initialNavigation;
@property BOOL ready;
@property BOOL closeReceived;
@property BOOL done;
@property BOOL bridgeRevoked;
@property BOOL budgetRevoked;
@property BOOL lifecycleRevoked;
@property BOOL foreignDenied;
@property NSUInteger navigationDenials;
@property NSUInteger rejected;
@property NSUInteger messages;
@property NSDictionary *theme;
@property NSUInteger pendingCalls;
@property NSInteger bridgeVersion;
@property NSDictionary *guestReport;
@property SinanUIFailure failure;
@property BOOL heartbeatPending;
@property BOOL heartbeatCompleted;
@property NSTimeInterval heartbeatStarted;
@property NSTimeInterval lastHeartbeat;
- (void)fail:(SinanUIFailure)reason;
- (void)checkResponsiveness;
- (void)applyTheme:(NSDictionary *)theme toWindow:(NSWindow *)window;

@end

@implementation SinanUIProbe
- (void)dealloc { if (_ownsControl && _control) sinanUIControlFree(_control); }
- (void)applyTheme:(NSDictionary *)theme toWindow:(NSWindow *)window {
    if (![theme isKindOfClass:NSDictionary.class] || [theme isEqual:self.theme]) return;
    self.theme = theme;
    self.header.theme = theme;
    [self.header refreshTheme];
    NSString *mode = theme[@"mode"];
    window.appearance = [mode isEqual:@"dark"] ? [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua] : [mode isEqual:@"light"] ? [NSAppearance appearanceNamed:NSAppearanceNameAqua] : nil;
    NSData *data = [NSJSONSerialization dataWithJSONObject:theme options:0 error:nil];
    NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    if (json) [self.view evaluateJavaScript:[NSString stringWithFormat:@"window.__sinanApplyTheme && window.__sinanApplyTheme(%@);",json] completionHandler:nil];
}
- (void)detachPanel:(id)sender { self.detachRequested = YES; }
- (void)backPanel:(id)sender {
    if (self.attached) self.backRequested = YES;
    else [self.view.window performClose:nil];
}
- (void)windowDidResignKey:(NSNotification *)notification {
    if (self.attached) [(NSWindow *)notification.object orderOut:nil];
}
- (void)fail:(SinanUIFailure)reason {
    if (self.done || self.revoked) return;
    self.failure = reason;
    self.bridgeRevoked = YES;
    self.done = YES;
    sinanUISessionRevoke(self.handle);
}
- (void)checkResponsiveness {
    if (!self.ready || self.done || self.revoked) return;
    NSTimeInterval now = NSProcessInfo.processInfo.systemUptime;
    if (self.heartbeatPending) {
        if (now - self.heartbeatStarted > 1.0) [self fail:SinanUIFailureUnresponsive];
        return;
    }
    if (now - self.lastHeartbeat < 0.25) return;
    self.heartbeatPending = YES;
    self.heartbeatStarted = now;
    __weak SinanUIProbe *weakSelf = self;
    [self.view evaluateJavaScript:@"true" completionHandler:^(id value, NSError *error) {
        SinanUIProbe *probe = weakSelf;
        if (!probe || probe.done || probe.revoked) return;
        probe.heartbeatPending = NO;
        probe.lastHeartbeat = NSProcessInfo.processInfo.systemUptime;
        if (error) [probe fail:SinanUIFailureScript];
        else probe.heartbeatCompleted = YES;
    }];
}
- (void)windowWillClose:(NSNotification *)notification {
    if (self.done) return;
    if (self.ready) {
        const char *close = "{\"kind\":\"close\"}";
        self.closeReceived = sinanUISessionMessage(self.handle, (char *)close, (int)strlen(close)) == 2;
    }
    sinanUISessionRevoke(self.handle);
    self.done = YES;
}
- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completionHandler { completionHandler(); }
- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))completionHandler { completionHandler(NO); }
- (void)webView:(WKWebView *)webView runJavaScriptTextInputPanelWithPrompt:(NSString *)prompt defaultText:(NSString *)defaultText initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSString *))completionHandler { completionHandler(nil); }
- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler { completionHandler(nil); }
- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type decisionHandler:(void (^)(WKPermissionDecision))decisionHandler { decisionHandler(WKPermissionDecisionDeny); }
- (BOOL)revoked { return self.handle == 0 || atomic_load(&self.control->revoked); }
- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
    NSString *url = task.request.URL.absoluteString;
    NSDictionary *resource = nil;
    if (!self.revoked && url.length <= 1024) {
        char *encoded = sinanUISessionResource(self.handle, (char *)url.UTF8String);
        if (encoded) {
            NSData *json = [NSData dataWithBytes:encoded length:strlen(encoded)];
            free(encoded);
            resource = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
        }
    }
    BOOL own = [task.request.URL.host isEqual:[NSURL URLWithString:self.entry].host];
    if (!own) self.foreignDenied = YES;
    NSData *data = resource ? [[NSData alloc] initWithBase64EncodedString:resource[@"data"] options:0] : [NSData data];
    if (!data) data = [NSData data];
    NSDictionary *headers = @{@"Content-Type":resource[@"mime"] ?: @"text/plain", @"Content-Security-Policy":self.csp, @"X-Content-Type-Options":@"nosniff", @"Cache-Control":@"no-store"};
    if (self.revoked) { resource = nil; data = [NSData data]; }
    NSHTTPURLResponse *response = [[NSHTTPURLResponse alloc] initWithURL:task.request.URL statusCode:self.revoked ? 410 : resource && own ? 200 : 404 HTTPVersion:@"HTTP/1.1" headerFields:headers];
    [task didReceiveResponse:response];
    [task didReceiveData:data];
    [task didFinish];
}
- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    // Only the single host-requested initial document may navigate. Decisions
    // cover all target frames, including nil targetFrame (new-window requests).
    BOOL initial = !self.revoked && !self.initialNavigation && action.targetFrame.isMainFrame && [action.request.URL.absoluteString isEqual:self.entry];
    if (initial) { self.initialNavigation = YES; decisionHandler(WKNavigationActionPolicyAllow); }
    else { self.navigationDenials++; decisionHandler(WKNavigationActionPolicyCancel); }
}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)response decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
    decisionHandler(!self.revoked && response.isForMainFrame && [response.response.URL.absoluteString isEqual:self.entry] ? WKNavigationResponsePolicyAllow : WKNavigationResponsePolicyCancel);
}
- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)action windowFeatures:(WKWindowFeatures *)features {
    self.navigationDenials++;
    return nil;
}
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView { [self fail:SinanUIFailureRenderer]; }
- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error { [self fail:SinanUIFailureNavigation]; }
- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error { [self fail:SinanUIFailureNavigation]; }
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    // Lifecycle/probe messages remain separate from the opt-in request bridge.
    if (self.bridgeRevoked || self.revoked) return;
    if (++self.messages > 32) {
        self.bridgeRevoked = YES;
        self.budgetRevoked = YES;
        self.done = YES;
        sinanUISessionRevoke(self.handle);
        [controller removeScriptMessageHandlerForName:@"sinan"];
        return;
    }
    if (!message.frameInfo.isMainFrame || ![message.frameInfo.request.URL.absoluteString isEqual:self.entry] || ![message.body isKindOfClass:NSDictionary.class]) { self.rejected++; return; }
    NSDictionary *body = message.body;
    if (![NSJSONSerialization isValidJSONObject:body]) { self.rejected++; return; }
    NSData *encoded = [NSJSONSerialization dataWithJSONObject:body options:0 error:nil];
    if (!encoded || encoded.length > 4096) { self.rejected++; return; }
    if ([body[@"kind"] isEqual:@"ready"] && body.count == 2 && !self.ready && [body[@"version"] isKindOfClass:NSNumber.class] && CFGetTypeID((__bridge CFTypeRef)body[@"version"]) != CFBooleanGetTypeID() && [body[@"version"] doubleValue] == self.bridgeVersion) {
        if (sinanUISessionMessage(self.handle, (char *)encoded.bytes, (int)encoded.length) != 1) { self.rejected++; return; }
        self.ready = YES;
        if (!self.probe) return;
        NSData *target = [NSJSONSerialization dataWithJSONObject:@[self.target] options:0 error:nil];
        NSString *argument = [[NSString alloc] initWithData:target encoding:NSUTF8StringEncoding];
        [self.view evaluateJavaScript:[NSString stringWithFormat:@"void window.runProbe((%@)[0]);", argument] completionHandler:^(id result, NSError *error) { if (error) [self fail:SinanUIFailureScript]; }];
        return;
    }
    if (self.probe && self.ready && [body[@"kind"] isEqual:@"report"] && body.count == 2 && !self.guestReport && [body[@"report"] isKindOfClass:NSDictionary.class]) {
        NSDictionary *report = body[@"report"];
        NSArray *keys = @[@"bindingsAbsent",@"storageIsolated",@"ownResourceLoaded",@"networkBlocked"];
        if (report.count != keys.count) { self.rejected++; return; }
        for (NSString *key in keys) {
            id value = report[key];
            if (!value || CFGetTypeID((__bridge CFTypeRef)value) != CFBooleanGetTypeID()) { self.rejected++; return; }
        }
        self.guestReport = report;
        return;
    }
    // Fixed diagnostic action only: it is not part of Session.Message and does
    // not accept a page-provided plugin ID or any other host operation.
    if (self.probe && self.ready && self.guestReport && [body[@"kind"] isEqual:@"revoke"] && body.count == 1) {
        self.lifecycleRevoked = sinanUIProbeDisable(self.handle) == 1 && self.revoked;
        self.bridgeRevoked = YES;
        self.done = YES;
        [controller removeScriptMessageHandlerForName:@"sinan"];
        return;
    }
    if (self.ready && (!self.probe || self.guestReport) && [body[@"kind"] isEqual:@"close"] && body.count == 1) {
        if (sinanUISessionMessage(self.handle, (char *)encoded.bytes, (int)encoded.length) == 2) { self.closeReceived = YES; self.done = YES; return; }
    } else {
        (void)sinanUISessionMessage(self.handle, (char *)encoded.bytes, (int)encoded.length);
    }
    self.rejected++;
}
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message replyHandler:(void (^)(id, NSString *))replyHandler {
    if (self.done || self.revoked || !self.ready || self.bridgeVersion != 2 || self.pendingCalls >= 1 || !message.frameInfo.isMainFrame || ![message.frameInfo.request.URL.absoluteString isEqual:self.entry] || ![message.body isKindOfClass:NSDictionary.class] || ![NSJSONSerialization isValidJSONObject:message.body]) { replyHandler(nil, @"UI 调用不可用"); return; }
    NSData *encoded = [NSJSONSerialization dataWithJSONObject:message.body options:0 error:nil];
    if (!encoded || encoded.length > 65536) { replyHandler(nil,@"UI 请求超过 64 KiB"); return; }
    self.pendingCalls++;
    unsigned long long handle = self.handle;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0), ^{
        char *raw = sinanUISessionCall(handle,(char *)encoded.bytes,(int)encoded.length);
        NSDictionary *response = nil;
        if (raw) { NSData *data = [NSData dataWithBytes:raw length:strlen(raw)]; response = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil]; free(raw); }
        dispatch_async(dispatch_get_main_queue(), ^{
            self.pendingCalls--;
            if (self.done || self.revoked) replyHandler(nil,@"UI 会话已关闭");
            else if (![response isKindOfClass:NSDictionary.class]) replyHandler(nil,@"UI 调用失败");
            else if (response[@"error"]) replyHandler(nil,response[@"error"]);
            else replyHandler(response[@"result"],nil);
        });
    });
}
@end

SinanUIProbeReport sinanUIProbe(const char *bundleJSON, const char *targetURL, unsigned long long handle, SinanUIControl *control) {
    @autoreleasepool {
        SinanUIProbeReport result = {0};
        NSData *data = [NSData dataWithBytes:bundleJSON length:strlen(bundleJSON)];
        NSDictionary *bundle = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (![bundle isKindOfClass:NSDictionary.class]) return result;
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:([bundle[@"probe"] boolValue] || [bundle[@"panel"] boolValue]) ? NSApplicationActivationPolicyAccessory : NSApplicationActivationPolicyRegular];
        [NSApp finishLaunching];
        SinanUIProbe *delegate = [SinanUIProbe new];
        delegate.entry = bundle[@"entry"];
        delegate.bridgeVersion = [bundle[@"bridgeVersion"] isEqual:@"2"] ? 2 : 1;
        delegate.probe = [bundle[@"probe"] boolValue];
        delegate.attached = [bundle[@"panel"] boolValue] && !delegate.probe;
        delegate.detachShortcut = @"Shift+Cmd+D";
        delegate.handle = handle;
        delegate.control = control;
        delegate.csp = bundle[@"csp"];
        delegate.target = [NSString stringWithUTF8String:targetURL];
        WKWebViewConfiguration *configuration = [WKWebViewConfiguration new];
        configuration.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
        NSData *themeData = [NSJSONSerialization dataWithJSONObject:bundle[@"theme"] options:0 error:nil];
        NSString *themeJSON = [[NSString alloc] initWithData:themeData encoding:NSUTF8StringEncoding];
        NSString *script = [NSString stringWithFormat:@"%@;window.__sinanApplyTheme(%@);", bundle[@"themeRuntime"],themeJSON];
        [configuration.userContentController addUserScript:[[WKUserScript alloc] initWithSource:script injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES]];
        [configuration setURLSchemeHandler:delegate forURLScheme:@"sinan-plugin"];
        [configuration.userContentController addScriptMessageHandler:delegate name:@"sinan"];
        if (delegate.bridgeVersion == 2) [configuration.userContentController addScriptMessageHandlerWithReply:delegate contentWorld:WKContentWorld.pageWorld name:@"sinanCall"];
        WKWebView *view = [[WKWebView alloc] initWithFrame:(delegate.probe ? NSMakeRect(0,0,560,240) : NSMakeRect(0,0,640,480)) configuration:configuration];
        delegate.view = view;
        view.navigationDelegate = delegate;
        view.UIDelegate = delegate;
        NSWindow *window;
        if (delegate.attached) {
            window = [[SinanMainPanel alloc] initWithContentRect:NSMakeRect(0,0,680,530) styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel backing:NSBackingStoreBuffered defer:NO];
            window.level = NSFloatingWindowLevel;
            window.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary;
            ((NSPanel *)window).hidesOnDeactivate = NO;
        } else {
            window = [[NSWindow alloc] initWithContentRect:view.frame styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable backing:NSBackingStoreBuffered defer:NO];
        }
        window.title = bundle[@"title"];
        window.delegate = delegate;
        [delegate applyTheme:bundle[@"theme"] toWindow:window];
        window.releasedWhenClosed = NO;
        if (delegate.attached) {
            NSView *container = [[NSView alloc] initWithFrame:NSMakeRect(0,0,680,530)];
            window.contentView = container;
            view.frame = NSMakeRect(0,0,680,530-SinanHeaderHeight);
            view.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
            [container addSubview:view];
            NSString *headerTitle = bundle[@"headerTitle"];
            if (!headerTitle.length) headerTitle = bundle[@"title"] ?: @"司南插件";
            SinanPanelHeader *header = [[SinanPanelHeader alloc] initWithTitle:headerTitle source:bundle[@"headerSource"] target:delegate];
            header.frame = NSMakeRect(0,530-SinanHeaderHeight,680,SinanHeaderHeight);
            header.theme = delegate.theme;
            [header refreshTheme];
            header.needsLayout = YES;
            header.detachButton.toolTip = detachTooltip(delegate.detachShortcut);
            delegate.header = header;
            delegate.detachButton = header.detachButton;
            [container addSubview:header];
        } else window.contentView = view;
        // Cocoa screen frames and mouseLocation share logical global coordinates,
        // including negative origins on monitors to the left/below the main one.
        NSScreen *targetScreen = NSScreen.mainScreen;
        NSPoint cursor = NSEvent.mouseLocation;
        for (NSScreen *screen in NSScreen.screens) {
            if (NSMouseInRect(cursor, screen.frame, NO)) { targetScreen = screen; break; }
        }
        if (targetScreen) {
            NSRect work = targetScreen.visibleFrame;
            NSRect frame = window.frame;
            frame.size.width = MIN(frame.size.width, work.size.width);
            frame.size.height = MIN(frame.size.height, work.size.height);
            frame.origin.x = NSMinX(work) + (work.size.width-frame.size.width)/2;
            frame.origin.y = NSMinY(work) + (work.size.height-frame.size.height)/2;
            [window setFrame:frame display:YES];
        } else [window center];
        if (delegate.attached) positionPanel(window);
        if (delegate.probe) [window orderFront:nil];
        else { [window makeKeyAndOrderFront:nil]; if (!delegate.attached) [NSApp activateIgnoringOtherApps:YES]; [window makeFirstResponder:view]; }
        [view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:delegate.entry]]];
        // A fixed, harmless event verifies that the AppKit queue is dispatched,
        // rather than merely servicing CF/NSRunLoop sources (e.g. WebKit IPC).
        const short marker = 0x534E;
        [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint modifierFlags:0 timestamp:NSProcessInfo.processInfo.systemUptime windowNumber:window.windowNumber context:nil subtype:marker data1:0 data2:0] atStart:NO];
        BOOL eventLoopResponsive = NO;
        int focusHandled = 0;
        int togglesHandled = 0;
        BOOL detached = NO;
        NSTimeInterval seconds = [bundle[@"deadlineSeconds"] doubleValue];
        NSTimeInterval deadline = seconds > 0 ? NSProcessInfo.processInfo.systemUptime + seconds : INFINITY;
        while (!delegate.done && NSProcessInfo.processInfo.systemUptime < deadline) {
            @autoreleasepool {
                // AppKit input/window events must be dequeued and dispatched.
                // NSRunLoop alone renders initial WebKit content but leaves the
                // window unresponsive to WindowServer, keyboard and mouse input.
                NSEvent *event = [NSApp nextEventMatchingMask:NSEventMaskAny untilDate:[NSDate dateWithTimeIntervalSinceNow:0.02] inMode:NSDefaultRunLoopMode dequeue:YES];
                if (delegate.attached) {
                    char *value = sinanUIDetachShortcut(handle);
                    if (value) {
                        NSString *shortcut = [NSString stringWithUTF8String:value];
                        free(value);
                        if (shortcut && ![shortcut isEqual:delegate.detachShortcut]) {
                            delegate.detachShortcut = shortcut;
                            delegate.detachButton.toolTip = detachTooltip(shortcut);
                        }
                    }
                }
                if (event) {
                    if (matchesDetachShortcut(event, @"Cmd+Comma", window)) {
                        sinanUIWindowAction(handle, 3);
                    } else if (delegate.attached && matchesDetachShortcut(event, delegate.detachShortcut, window))
                        delegate.detachRequested = YES;
                    else [NSApp sendEvent:event];
                    if (event.type == NSEventTypeApplicationDefined && event.subtype == marker) eventLoopResponsive = YES;
                }
                [NSApp updateWindows];
            }
            if (delegate.ready && !delegate.done && !delegate.revoked) {
                unsigned long long controlID = 0;
                int kind = sinanUIPollControl(handle, &controlID);
                if (kind == 3) delegate.detachRequested = YES;
                if (kind == 1 || kind == 2) {
                    if (kind == 2 && delegate.attached && window.visible) [window orderOut:nil];
                    else {
                        if (delegate.attached) positionPanel(window);
                        if (window.miniaturized) [window deminiaturize:nil];
                        [window makeKeyAndOrderFront:nil];
                        if (!delegate.attached) [NSApp activateIgnoringOtherApps:YES];
                        [window makeFirstResponder:view];
                    }
                }
                if (delegate.backRequested && delegate.attached) {
                    delegate.backRequested = NO;
                    if (sinanUIWindowAction(handle, 2)) delegate.done = YES;
                }
                if (delegate.detachRequested && delegate.attached) {
                    delegate.detachRequested = NO;
                    if (sinanUIWindowAction(handle, 1)) {
                        NSWindow *previous = window;
                        delegate.attached = NO;
                        // Move the same content tree, keeping both React state and
                        // host-owned chrome just like the trusted management view.
                        NSView *content = previous.contentView;
                        NSRect contentFrame = content.bounds;
                        previous.contentView = [[NSView alloc] initWithFrame:contentFrame];
                        delegate.header.detached = YES;
                        delegate.header.backButton.title = @"×";
                        [delegate.header.backButton setAccessibilityLabel:@"关闭窗口"];
                        delegate.header.detachButton.hidden = YES;
                        [delegate.header refreshTheme];
                        delegate.header.needsLayout = YES;
                        window = [[NSWindow alloc] initWithContentRect:contentFrame styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable backing:NSBackingStoreBuffered defer:NO];
                        window.releasedWhenClosed = NO;
                        window.title = bundle[@"title"];
                        window.delegate = delegate;
                        window.contentView = content;
                        window.appearance = previous.appearance;
                        NSRect previousFrame = previous.frame;
                        NSRect work = previous.screen.visibleFrame;
                        previous.delegate = nil;
                        [previous close];
                        NSRect frame = window.frame;
                        frame.origin.x = NSMidX(previousFrame) - frame.size.width / 2;
                        frame.origin.y = NSMaxY(previousFrame) - frame.size.height;
                        frame.origin.x = MAX(NSMinX(work), MIN(frame.origin.x, NSMaxX(work) - frame.size.width));
                        frame.origin.y = MAX(NSMinY(work), MIN(frame.origin.y, NSMaxY(work) - frame.size.height));
                        [window setFrame:frame display:YES];
                        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
                        [window makeKeyAndOrderFront:nil];
                        [NSApp activateIgnoringOtherApps:YES];
                        [window makeFirstResponder:view];
                        detached = YES;
                    }
                }
                if (kind && !delegate.revoked && sinanUIFocused(handle, controlID)) {
                    if (kind == 1) focusHandled++;
                    if (kind == 2) togglesHandled++;
                }
            }
            char *encodedTheme = sinanUITheme(handle,[delegate.theme[@"revision"] unsignedLongLongValue]);
            if (encodedTheme) {
                NSData *data = [NSData dataWithBytes:encodedTheme length:strlen(encodedTheme)]; free(encodedTheme);
                NSDictionary *theme = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
                [delegate applyTheme:theme toWindow:window];
            }
            [delegate checkResponsiveness];
            if (delegate.revoked && !delegate.done) {
                delegate.lifecycleRevoked = YES;
                delegate.bridgeRevoked = YES;
                delegate.done = YES;
                [configuration.userContentController removeScriptMessageHandlerForName:@"sinan"];
            }
        }
        if (!delegate.done) [delegate fail:SinanUIFailureDeadline];
        result.eventLoopResponsive = eventLoopResponsive;
        result.focusHandled = focusHandled;
        result.detached = detached;
        result.togglesHandled = togglesHandled;
        result.failure = delegate.failure;
        result.heartbeatCompleted = delegate.heartbeatCompleted;
        result.ready = delegate.ready;
        result.bindingsAbsent = [delegate.guestReport[@"bindingsAbsent"] boolValue];
        result.storageIsolated = !configuration.websiteDataStore.persistent && [delegate.guestReport[@"storageIsolated"] boolValue];
        result.ownResourceLoaded = [delegate.guestReport[@"ownResourceLoaded"] boolValue];
        result.networkBlocked = [delegate.guestReport[@"networkBlocked"] boolValue];
        result.foreignResourceBlocked = delegate.foreignDenied;
        result.navigationBlocked = delegate.navigationDenials >= 2;
        result.invalidMessageRejected = delegate.rejected >= 3;
        result.closeReceived = delegate.closeReceived;
        result.budgetRevoked = delegate.budgetRevoked;
        result.lifecycleRevoked = delegate.lifecycleRevoked;
        sinanUISessionRevoke(handle);
        // Late WebKit callbacks cannot dereference the Go handle after return.
        // Drain already-admitted work before releasing the Go handle. The private
        // client has a one-second deadline; revocation unblocks pending calls.
        delegate.done = YES;
        [configuration.userContentController removeScriptMessageHandlerForName:@"sinanCall" contentWorld:WKContentWorld.pageWorld];
        while (delegate.pendingCalls > 0) {
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
        }
        delegate.handle = 0;
        [view stopLoading];
        [configuration.userContentController removeScriptMessageHandlerForName:@"sinan"];
        view.navigationDelegate = nil;
        view.UIDelegate = nil;
        delegate.view = nil;
        window.delegate = nil;
        [window close];
        return result;
    }
}


SinanEmbeddedRect sinanUIEmbeddedRect(double width, double height, int detached, int flipped) {
    double available = MAX(0,height-58);
    double body = detached ? available : MIN(430,available);
    return (SinanEmbeddedRect){0,flipped ? MIN(58,MAX(0,height)) : MAX(0,height-58-body),MAX(0,width),body};
}

@interface SinanEmbeddedWebView : WKWebView
@property BOOL growWithWindow;
@end
@implementation SinanEmbeddedWebView
- (void)resizeWithOldSuperviewSize:(NSSize)oldSize {
    NSView *parent = self.superview;
    if (!parent) return;
    SinanEmbeddedRect rect = sinanUIEmbeddedRect(parent.bounds.size.width,parent.bounds.size.height,self.growWithWindow,parent.isFlipped);
    self.frame = NSMakeRect(rect.x,rect.y,rect.width,rect.height);
}
@end

// An isolated WebView in the existing host window. Wails' configuration and
// scripts are deliberately not reused: this page receives only the narrow bridge.
void *sinanUIEmbeddedCreate(const char *json, unsigned long long handle, void *nativeWindow) {
    @autoreleasepool {
        NSDictionary *bundle = [NSJSONSerialization JSONObjectWithData:[[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
        NSWindow *window = (__bridge NSWindow *)nativeWindow;
        if (!window || ![bundle isKindOfClass:NSDictionary.class]) return NULL;
        SinanUIProbe *delegate = [SinanUIProbe new];
        delegate.handle = handle;
        delegate.control = sinanUIControlNew();
        if (!delegate.control) return NULL;
        delegate.ownsControl = YES;
        delegate.entry = bundle[@"entry"];
        delegate.csp = bundle[@"csp"];
        delegate.bridgeVersion = [bundle[@"bridgeVersion"] isEqual:@"2"] ? 2 : 1;
        WKWebViewConfiguration *configuration = [WKWebViewConfiguration new];
        configuration.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
        NSData *themeData = [NSJSONSerialization dataWithJSONObject:bundle[@"theme"] options:0 error:nil];
        NSString *themeJSON = [[NSString alloc] initWithData:themeData encoding:NSUTF8StringEncoding];
        NSString *script = [NSString stringWithFormat:@"%@;window.__sinanApplyTheme(%@);",bundle[@"themeRuntime"],themeJSON];
        [configuration.userContentController addUserScript:[[WKUserScript alloc] initWithSource:script injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES]];
        [configuration setURLSchemeHandler:delegate forURLScheme:@"sinan-plugin"];
        [configuration.userContentController addScriptMessageHandler:delegate name:@"sinan"];
        if (delegate.bridgeVersion == 2) [configuration.userContentController addScriptMessageHandlerWithReply:delegate contentWorld:WKContentWorld.pageWorld name:@"sinanCall"];
        NSView *parent = window.contentView;
        NSRect frame = parent.bounds;
        frame.size.height = MAX(0,frame.size.height - 58);
        if (parent.isFlipped) frame.origin.y += 58;
        WKWebView *view = [[SinanEmbeddedWebView alloc] initWithFrame:frame configuration:configuration];
        view.hidden = YES;
        view.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
        view.navigationDelegate = delegate;
        view.UIDelegate = delegate;
        delegate.view = view;
        [parent addSubview:view positioned:NSWindowAbove relativeTo:nil];
        [delegate applyTheme:bundle[@"theme"] toWindow:window];
        delegate.detachShortcut = bundle[@"detachShortcut"] ?: @"Shift+Cmd+D";
        __weak SinanUIProbe *weak = delegate;
        delegate.embeddedKeyMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *event) {
            SinanUIProbe *current = weak;
            if (!current || current.revoked || current.view.hidden || current.view.window != event.window) return event;
            if (matchesDetachShortcut(event,@"Cmd+Comma",event.window)) { sinanUIEmbeddedAction(current.handle,2); return nil; }
            if (matchesDetachShortcut(event,current.detachShortcut,event.window)) { sinanUIEmbeddedAction(current.handle,1); return nil; }
            return event;
        }];
        parent.postsFrameChangedNotifications = YES;
        delegate.embeddedResizeObserver = [NSNotificationCenter.defaultCenter addObserverForName:NSViewFrameDidChangeNotification object:parent queue:nil usingBlock:^(NSNotification *note) {
            SinanUIProbe *current = weak;
            if (current && !current.revoked) [current.view resizeWithOldSuperviewSize:NSZeroSize];
        }];
        delegate.embeddedTimer = [NSTimer scheduledTimerWithTimeInterval:0.1 repeats:YES block:^(NSTimer *timer) {
            SinanUIProbe *current = weak;
            if (!current) { [timer invalidate]; return; }
            if (current.done && current.handle) sinanUISessionRevoke(current.handle);
            else if (!current.revoked) [current checkResponsiveness];
        }];
        [view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:delegate.entry]]];
        return (void *)CFBridgingRetain(delegate);
    }
}
int sinanUIEmbeddedStatus(void *value) {
    SinanUIProbe *delegate = (__bridge SinanUIProbe *)value;
    return delegate.done || delegate.revoked ? -1 : delegate.ready ? 1 : 0;
}
void sinanUIEmbeddedShow(void *value) {
    SinanUIProbe *delegate = (__bridge SinanUIProbe *)value;
    delegate.view.hidden = NO;
    [delegate.view.window makeFirstResponder:delegate.view];
}
void sinanUIEmbeddedTheme(void *value, const char *json) {
    SinanUIProbe *delegate = (__bridge SinanUIProbe *)value;
    NSDictionary *theme = [NSJSONSerialization JSONObjectWithData:[[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
    [delegate applyTheme:theme toWindow:delegate.view.window];
}
void sinanUIEmbeddedClose(void *value) {
    SinanUIProbe *delegate = CFBridgingRelease(value);
    sinanUIControlRevoke(delegate.control);
    delegate.handle = 0;
    if (delegate.embeddedKeyMonitor) [NSEvent removeMonitor:delegate.embeddedKeyMonitor];
    delegate.embeddedKeyMonitor = nil;
    if (delegate.embeddedResizeObserver) [NSNotificationCenter.defaultCenter removeObserver:delegate.embeddedResizeObserver];
    delegate.embeddedResizeObserver = nil;
    [delegate.embeddedTimer invalidate];
    delegate.embeddedTimer = nil;
    WKWebView *view = delegate.view;
    [view.configuration.userContentController removeScriptMessageHandlerForName:@"sinan"];
    [view.configuration.userContentController removeScriptMessageHandlerForName:@"sinanCall" contentWorld:WKContentWorld.pageWorld];
    view.navigationDelegate = nil;
    view.UIDelegate = nil;
    [view stopLoading];
    NSWindow *window = view.window;
    id responder = window.firstResponder;
    if ([responder isKindOfClass:NSView.class] && (responder == view || [(NSView *)responder isDescendantOf:view])) [window makeFirstResponder:nil];
    [view removeFromSuperview];
    delegate.view = nil;
}

void sinanUIEmbeddedShortcut(void *value, const char *shortcut) {
    SinanUIProbe *delegate = (__bridge SinanUIProbe *)value;
    delegate.detachShortcut = [NSString stringWithUTF8String:shortcut];
}

void sinanUIEmbeddedDetached(void *value) {
    SinanUIProbe *delegate = (__bridge SinanUIProbe *)value;
    ((SinanEmbeddedWebView *)delegate.view).growWithWindow = YES;
    [delegate.view resizeWithOldSuperviewSize:NSZeroSize];
}
