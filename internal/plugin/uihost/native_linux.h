//go:build linux && cgo && gtk3

#ifndef SINAN_GTK_UI_H
#define SINAN_GTK_UI_H
#include <webkit2/webkit2.h>
#if !WEBKIT_CHECK_VERSION(2, 40, 0)
#error "Sinan GTK plugin UI requires WebKitGTK 2.40 or newer"
#endif

typedef struct SinanGTKSurface SinanGTKSurface;
SinanGTKSurface *sinanGTKCreate(void *window, unsigned long long handle, const char *entry, const char *csp, const char *pageScript, const char *isolatedScript, const char *themeScript, int bridgeVersion);
void sinanGTKClose(SinanGTKSurface *surface);
int sinanGTKStatus(SinanGTKSurface *surface);
void sinanGTKShow(SinanGTKSurface *surface);
void sinanGTKTheme(SinanGTKSurface *surface, const char *script);
void sinanGTKShortcut(SinanGTKSurface *surface, const char *shortcut);
void sinanGTKDetached(SinanGTKSurface *surface);
int sinanGTKBodyHeight(int height, int detached);
int sinanGTKShortcutMatches(char *shortcut, char *key, unsigned int modifiers, unsigned int hardware);
void sinanGTKReply(WebKitWebView *view, WebKitScriptMessageReply *reply, JSCContext *context, const char *data, int failed);

extern void *sinanGTKResource(unsigned long long handle, char *method, char *url, char **mime, int *length);
extern int sinanGTKMessage(unsigned long long handle, char *source, char *data, int length);
extern int sinanGTKCall(unsigned long long handle, char *source, char *data, int length, WebKitWebView *view, WebKitScriptMessageReply *reply, JSCContext *context);
extern int sinanGTKNavigate(unsigned long long handle, char *target);
extern void sinanGTKRevoke(unsigned long long handle);
extern void sinanGTKAction(unsigned long long handle, int kind);
#endif
