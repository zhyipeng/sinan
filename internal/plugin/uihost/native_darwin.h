//go:build darwin && !ios && cgo

#ifndef SINAN_UI_PROBE_H
#define SINAN_UI_PROBE_H
typedef enum { SinanUIFailureNone, SinanUIFailureScript, SinanUIFailureRenderer, SinanUIFailureNavigation, SinanUIFailureUnresponsive, SinanUIFailureDeadline } SinanUIFailure;
typedef struct {
    int ready, bindingsAbsent, storageIsolated, ownResourceLoaded;
    int foreignResourceBlocked, navigationBlocked, networkBlocked;
    int invalidMessageRejected, closeReceived, budgetRevoked, lifecycleRevoked;
    int failure, heartbeatCompleted, eventLoopResponsive, focusHandled, detached, togglesHandled;
} SinanUIProbeReport;
typedef struct SinanUIControl SinanUIControl;
SinanUIControl *sinanUIControlNew(void);
void sinanUIControlRevoke(SinanUIControl *control);
void sinanUIControlFree(SinanUIControl *control);
SinanUIProbeReport sinanUIProbe(const char *metadataJSON, const char *targetURL, unsigned long long handle, SinanUIControl *control);
extern char *sinanUISessionResource(unsigned long long handle, char *method, char *url);
extern char *sinanUISessionCall(unsigned long long handle, char *source, int mainFrame, char *json, int length);
extern int sinanUISessionMessage(unsigned long long handle, char *source, int mainFrame, char *json, int length);
extern int sinanUISessionNavigate(unsigned long long handle, char *target, int mainFrame);
extern void sinanUISessionRevoke(unsigned long long handle);
extern int sinanUIProbeDisable(unsigned long long handle);
int sinanUIDetachKeyMatches(char *shortcut, unsigned short keyCode, unsigned long modifiers);
extern char *sinanUITheme(unsigned long long handle, unsigned long long revision);
extern char *sinanUIDetachShortcut(unsigned long long handle);
extern int sinanUIPollControl(unsigned long long handle, unsigned long long *id);
extern int sinanUIWindowAction(unsigned long long handle, int kind);
extern int sinanUIFocused(unsigned long long handle, unsigned long long id);
void *sinanUIEmbeddedCreate(const char *json, unsigned long long handle, void *window);
int sinanUIEmbeddedStatus(void *value);
void sinanUIEmbeddedShow(void *value);
void sinanUIEmbeddedTheme(void *value, const char *json);
void sinanUIEmbeddedClose(void *value);
void sinanUIEmbeddedShortcut(void *value, const char *shortcut);
extern void sinanUIEmbeddedAction(unsigned long long handle, int action);
void sinanUIEmbeddedDetached(void *value);
typedef struct { double x,y,width,height; } SinanEmbeddedRect;
SinanEmbeddedRect sinanUIEmbeddedRect(double width, double height, int detached, int flipped);
#endif
