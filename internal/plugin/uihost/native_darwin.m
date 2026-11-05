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

@interface SinanUIProbe : NSObject <WKNavigationDelegate, WKUIDelegate, WKURLSchemeHandler, WKScriptMessageHandler, NSWindowDelegate>
@property unsigned long long handle;
@property SinanUIControl *control;
@property NSString *entry;
@property NSString *csp;
@property NSString *target;
@property WKWebView *view;
@property BOOL probe;
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
@property NSDictionary *guestReport;
@property SinanUIFailure failure;
@property BOOL heartbeatPending;
@property BOOL heartbeatCompleted;
@property NSTimeInterval heartbeatStarted;
@property NSTimeInterval lastHeartbeat;
- (void)fail:(SinanUIFailure)reason;
- (void)checkResponsiveness;

@end

@implementation SinanUIProbe
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
    // This handler exists only in the development host. It has no filesystem,
    // command execution, settings or other Wails/host API to dispatch to.
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
    if ([body[@"kind"] isEqual:@"ready"] && body.count == 2 && !self.ready && [body[@"version"] isKindOfClass:NSNumber.class] && CFGetTypeID((__bridge CFTypeRef)body[@"version"]) != CFBooleanGetTypeID() && [body[@"version"] doubleValue] == 1) {
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
@end

SinanUIProbeReport sinanUIProbe(const char *bundleJSON, const char *targetURL, unsigned long long handle, SinanUIControl *control) {
    @autoreleasepool {
        SinanUIProbeReport result = {0};
        NSData *data = [NSData dataWithBytes:bundleJSON length:strlen(bundleJSON)];
        NSDictionary *bundle = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (![bundle isKindOfClass:NSDictionary.class]) return result;
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:[bundle[@"probe"] boolValue] ? NSApplicationActivationPolicyAccessory : NSApplicationActivationPolicyRegular];
        [NSApp finishLaunching];
        SinanUIProbe *delegate = [SinanUIProbe new];
        delegate.entry = bundle[@"entry"];
        delegate.probe = [bundle[@"probe"] boolValue];
        delegate.handle = handle;
        delegate.control = control;
        delegate.csp = bundle[@"csp"];
        delegate.target = [NSString stringWithUTF8String:targetURL];
        WKWebViewConfiguration *configuration = [WKWebViewConfiguration new];
        configuration.websiteDataStore = WKWebsiteDataStore.nonPersistentDataStore;
        [configuration setURLSchemeHandler:delegate forURLScheme:@"sinan-plugin"];
        [configuration.userContentController addScriptMessageHandler:delegate name:@"sinan"];
        WKWebView *view = [[WKWebView alloc] initWithFrame:NSMakeRect(0,0,560,240) configuration:configuration];
        delegate.view = view;
        view.navigationDelegate = delegate;
        view.UIDelegate = delegate;
        NSWindow *window = [[NSWindow alloc] initWithContentRect:view.frame styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable backing:NSBackingStoreBuffered defer:NO];
        window.title = bundle[@"title"];
        window.delegate = delegate;
        window.releasedWhenClosed = NO;
        window.contentView = view;
        [window center];
        if (delegate.probe) [window orderFront:nil];
        else { [window makeKeyAndOrderFront:nil]; [NSApp activateIgnoringOtherApps:YES]; [window makeFirstResponder:view]; }
        [view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:delegate.entry]]];
        NSTimeInterval seconds = [bundle[@"deadlineSeconds"] doubleValue];
        NSTimeInterval deadline = seconds > 0 ? NSProcessInfo.processInfo.systemUptime + seconds : INFINITY;
        while (!delegate.done && NSProcessInfo.processInfo.systemUptime < deadline) {
            [[NSRunLoop mainRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.02]];
            [delegate checkResponsiveness];
            if (delegate.revoked && !delegate.done) {
                delegate.lifecycleRevoked = YES;
                delegate.bridgeRevoked = YES;
                delegate.done = YES;
                [configuration.userContentController removeScriptMessageHandlerForName:@"sinan"];
            }
        }
        if (!delegate.done) [delegate fail:SinanUIFailureDeadline];
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
