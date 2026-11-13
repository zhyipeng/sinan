//go:build linux && cgo && gtk3

#include "native_linux.h"
#include <stdlib.h>
#include <string.h>
#include <gdk/gdkkeysyms.h>
#include <linux/input-event-codes.h>

#define SINAN_WORLD "sinan-isolated-host"
#define SINAN_DATA "sinan-plugin-surface"

struct SinanGTKSurface {
    WebKitWebView *view;
    GtkWidget *overlay;
    unsigned long long handle;
    gchar *entry, *shortcut;
    GCancellable *cancellable;
    gboolean ready, failed, closed, detached, navigating, pingPending;
    guint timer;
    gint64 pingStarted, lastPing;
    guint pendingCalls;
    guint messages, callAttempts;
    int bridgeVersion;
};

static void surfaceFree(gpointer data) {
    SinanGTKSurface *s = data;
    g_free(s->entry); g_free(s->shortcut); g_clear_object(&s->cancellable); g_free(s);
}
static gboolean active(SinanGTKSurface *s) { return s && !s->closed && !s->failed && s->handle != 0; }
static gboolean ownDocument(SinanGTKSurface *s) {
    return active(s) && g_strcmp0(webkit_web_view_get_uri(s->view),s->entry) == 0;
}
static void failSurface(SinanGTKSurface *s) {
    if (!active(s)) return;
    s->failed = TRUE;
    sinanGTKRevoke(s->handle);
}

typedef struct { unsigned long long handle; gchar *csp; } SchemeData;
static void schemeFree(gpointer data) { SchemeData *scheme = data; g_free(scheme->csp); g_free(scheme); }
static void schemeRequest(WebKitURISchemeRequest *request, gpointer data) {
    SchemeData *scheme = data;
    unsigned long long handle = scheme->handle;
    const gchar *uri = webkit_uri_scheme_request_get_uri(request);
    const gchar *method = webkit_uri_scheme_request_get_http_method(request);
    char *mime = NULL; int length = 0;
    void *bytes = uri && strlen(uri) <= 1024 ? sinanGTKResource(handle,(char *)(method ? method : ""),(char *)uri,&mime,&length) : NULL;
    gboolean found = bytes != NULL;
    GInputStream *stream = g_memory_input_stream_new_from_data(bytes,found ? length : 0,g_free);
    WebKitURISchemeResponse *response = webkit_uri_scheme_response_new(stream,found ? length : 0);
    webkit_uri_scheme_response_set_status(response,found ? 200 : 404,found ? "OK" : "Not Found");
    webkit_uri_scheme_response_set_content_type(response,mime ? mime : "text/plain");
    SoupMessageHeaders *headers = soup_message_headers_new(SOUP_MESSAGE_HEADERS_RESPONSE);
    // Keep the exact policy on every response, including denied requests.
    soup_message_headers_append(headers,"Content-Security-Policy",scheme->csp);
    soup_message_headers_append(headers,"Cache-Control","no-store");
    soup_message_headers_append(headers,"X-Content-Type-Options","nosniff");
    webkit_uri_scheme_response_set_http_headers(response,headers); // Takes ownership.
    webkit_uri_scheme_request_finish_with_response(request,response);
    g_object_unref(response); g_object_unref(stream); free(mime);
}

static gboolean navigation(WebKitWebView *view, WebKitPolicyDecision *decision, WebKitPolicyDecisionType type, gpointer data) {
    SinanGTKSurface *s = data;
    gboolean allow = FALSE;
    if (active(s) && type == WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION && !s->navigating) {
        WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
        WebKitURIRequest *request = webkit_navigation_action_get_request(action);
        const gchar *target = webkit_uri_request_get_uri(request);
        // No page code exists yet. This one target is the host-requested load,
        // not a URL, frame identity, redirect or popup supplied by a plugin.
        if (target && strlen(target) <= 1024 && !webkit_navigation_action_is_redirect(action) && g_strcmp0(target,s->entry) == 0)
            allow = sinanGTKNavigate(s->handle,(char *)target) == 1;
        if (allow) s->navigating = TRUE;
    } else if (active(s) && type == WEBKIT_POLICY_DECISION_TYPE_RESPONSE) {
        WebKitResponsePolicyDecision *response = WEBKIT_RESPONSE_POLICY_DECISION(decision);
        allow = webkit_response_policy_decision_is_main_frame_main_resource(response) &&
            g_strcmp0(webkit_uri_response_get_uri(webkit_response_policy_decision_get_response(response)),s->entry) == 0;
    }
    if (allow) webkit_policy_decision_use(decision); else webkit_policy_decision_ignore(decision);
    return TRUE;
}
static WebKitWebView *newWindow(WebKitWebView *view, WebKitNavigationAction *action, gpointer data) { return NULL; }
static gboolean permission(WebKitWebView *view, WebKitPermissionRequest *request, gpointer data) { webkit_permission_request_deny(request); return TRUE; }
static gboolean chooser(WebKitWebView *view, WebKitFileChooserRequest *request, gpointer data) { webkit_file_chooser_request_cancel(request); return TRUE; }
static gboolean dialog(WebKitWebView *view, WebKitScriptDialog *dialog, gpointer data) { webkit_script_dialog_close(dialog); return TRUE; }
static gboolean contextMenu(WebKitWebView *view, WebKitContextMenu *menu, GdkEvent *event, WebKitHitTestResult *hit, gpointer data) { return TRUE; }
static void download(WebKitWebContext *context, WebKitDownload *download, gpointer data) { webkit_download_cancel(download); }
static gboolean loadFailed(WebKitWebView *view, WebKitLoadEvent event, gchar *uri, GError *error, gpointer data) { failSurface(data); return TRUE; }
static void rendererFailed(WebKitWebView *view, WebKitWebProcessTerminationReason reason, gpointer data) { failSurface(data); }
static void viewDestroyed(GtkWidget *view, gpointer data) { failSurface(data); }

static void message(WebKitUserContentManager *manager, WebKitJavascriptResult *value, gpointer data) {
    SinanGTKSurface *s = data;
    if (!ownDocument(s)) return;
    if (++s->messages > 32) { failSurface(s); return; }
    gchar *json = jsc_value_to_json(webkit_javascript_result_get_js_value(value),0);
    if (!json) return;
    int action = strlen(json) <= 4096 ? sinanGTKMessage(s->handle,s->entry,json,(int)strlen(json)) : 0;
    if (action == 1) s->ready = TRUE;
    if (action == 2) s->failed = TRUE;
    g_free(json);
}
static gboolean call(WebKitUserContentManager *manager, JSCValue *value, WebKitScriptMessageReply *reply, gpointer data) {
    SinanGTKSurface *s = data;
    if (active(s) && ++s->callAttempts > 256) failSurface(s);
    if (!ownDocument(s) || !s->ready || s->bridgeVersion != 2 || s->pendingCalls != 0) {
        webkit_script_message_reply_return_error_message(reply,"UI 调用不可用"); return TRUE;
    }
    gchar *json = jsc_value_to_json(value,0);
    if (!json || strlen(json) > 65536) {
        webkit_script_message_reply_return_error_message(reply,"UI 请求无效或超限"); g_free(json); return TRUE;
    }
    WebKitScriptMessageReply *ownedReply = webkit_script_message_reply_ref(reply);
    JSCContext *context = g_object_ref(jsc_value_get_context(value));
    g_object_ref(s->view);
    s->pendingCalls++;
    if (!sinanGTKCall(s->handle,s->entry,json,(int)strlen(json),s->view,ownedReply,context)) {
        sinanGTKReply(s->view,ownedReply,context,"null",1);
    }
    g_free(json);
    return TRUE;
}
void sinanGTKReply(WebKitWebView *view, WebKitScriptMessageReply *reply, JSCContext *context, const char *data, int failed) {
    SinanGTKSurface *s = g_object_get_data(G_OBJECT(view),SINAN_DATA);
    if (s && s->pendingCalls) s->pendingCalls--;
    if (!ownDocument(s) || failed) webkit_script_message_reply_return_error_message(reply,"UI 调用失败或会话已关闭");
    else {
        JSCValue *value = jsc_value_new_from_json(context,data);
        if (value) { webkit_script_message_reply_return_value(reply,value); g_object_unref(value); }
        else webkit_script_message_reply_return_error_message(reply,"UI 返回值无效");
    }
    webkit_script_message_reply_unref(reply); g_object_unref(context); g_object_unref(view);
}

int sinanGTKBodyHeight(int height, int detached) {
    if (height <= 58) return 0;
    int available = height-58;
    return detached ? available : MIN(542,available);
}
static void resized(GtkWidget *overlay, GtkAllocation *allocation, gpointer data) {
    SinanGTKSurface *s = data;
    if (s->closed) return;
    int height = sinanGTKBodyHeight(allocation->height,s->detached);
    gtk_widget_set_size_request(GTK_WIDGET(s->view),-1,height);
}
static guint canonicalKey(guint key) {
    switch (key) {
    case GDK_KEY_ISO_Left_Tab: return GDK_KEY_Tab;
    case GDK_KEY_KP_Enter: return GDK_KEY_Return;
    case GDK_KEY_KP_Left: return GDK_KEY_Left;
    case GDK_KEY_KP_Right: return GDK_KEY_Right;
    case GDK_KEY_KP_Up: return GDK_KEY_Up;
    case GDK_KEY_KP_Down: return GDK_KEY_Down;
    default: return gdk_keyval_to_lower(key);
    }
}
// GTK X11/Wayland use the standard XKB keycode (evdev code + 8).
// Match the physical Key/Digit positions recorded by KeyboardEvent.code,
// including shifted digits and non-Latin layouts; use keyvals for other keys.
static guint eventKey(GdkEventKey *event) {
    if (event->hardware_keycode >= 8) {
        switch (event->hardware_keycode - 8) {
#define PHYSICAL(code, name) case KEY_##code: return GDK_KEY_##name
        PHYSICAL(A,a);
        PHYSICAL(B,b);
        PHYSICAL(C,c);
        PHYSICAL(D,d);
        PHYSICAL(E,e);
        PHYSICAL(F,f);
        PHYSICAL(G,g);
        PHYSICAL(H,h);
        PHYSICAL(I,i);
        PHYSICAL(J,j);
        PHYSICAL(K,k);
        PHYSICAL(L,l);
        PHYSICAL(M,m);
        PHYSICAL(N,n);
        PHYSICAL(O,o);
        PHYSICAL(P,p);
        PHYSICAL(Q,q);
        PHYSICAL(R,r);
        PHYSICAL(S,s);
        PHYSICAL(T,t);
        PHYSICAL(U,u);
        PHYSICAL(V,v);
        PHYSICAL(W,w);
        PHYSICAL(X,x);
        PHYSICAL(Y,y);
        PHYSICAL(Z,z);
        PHYSICAL(0,0);
        PHYSICAL(1,1);
        PHYSICAL(2,2);
        PHYSICAL(3,3);
        PHYSICAL(4,4);
        PHYSICAL(5,5);
        PHYSICAL(6,6);
        PHYSICAL(7,7);
        PHYSICAL(8,8);
        PHYSICAL(9,9);
        PHYSICAL(COMMA,comma);
#undef PHYSICAL
        default: break;
        }
    }
    return canonicalKey(event->keyval);
}
static gboolean shortcutMatches(const char *shortcut, GdkEventKey *event) {
    if (!shortcut || !*shortcut) return FALSE;
    gchar **parts = g_strsplit(shortcut,"+",-1);
    GdkModifierType modifiers = 0; guint key = 0;
    for (int i=0; parts[i]; i++) {
        const char *part = parts[i];
        if (!strcmp(part,"Ctrl")) modifiers |= GDK_CONTROL_MASK;
        else if (!strcmp(part,"Alt")) modifiers |= GDK_MOD1_MASK;
        else if (!strcmp(part,"Shift")) modifiers |= GDK_SHIFT_MASK;
        else if (!strcmp(part,"Super") || !strcmp(part,"Cmd")) modifiers |= GDK_SUPER_MASK;
        else if (!strcmp(part,"Comma")) key = GDK_KEY_comma;
        else if (!strcmp(part,"Space")) key = GDK_KEY_space;
        else if (!strcmp(part,"Enter")) key = GDK_KEY_Return;
        else if (!strcmp(part,"Backspace")) key = GDK_KEY_BackSpace;
        else if (!strcmp(part,"Page Up")) key = GDK_KEY_Page_Up;
        else if (!strcmp(part,"Page Down")) key = GDK_KEY_Page_Down;
        else if (!strcmp(part,"Left")) key = GDK_KEY_Left;
        else if (!strcmp(part,"Right")) key = GDK_KEY_Right;
        else if (!strcmp(part,"Up")) key = GDK_KEY_Up;
        else if (!strcmp(part,"Down")) key = GDK_KEY_Down;
        else key = gdk_keyval_from_name(part);
    }
    g_strfreev(parts);
    GdkModifierType actual = event->state & (GDK_CONTROL_MASK|GDK_MOD1_MASK|GDK_SHIFT_MASK|GDK_SUPER_MASK|GDK_META_MASK|GDK_HYPER_MASK|GDK_MOD3_MASK|GDK_MOD5_MASK);
    return key && actual == modifiers && eventKey(event) == canonicalKey(key);
}
int sinanGTKShortcutMatches(char *shortcut, char *key, unsigned int modifiers, unsigned int hardware) {
    GdkEventKey event = {0}; event.keyval=gdk_keyval_from_name(key); event.state=modifiers; event.hardware_keycode=hardware;
    return shortcutMatches(shortcut,&event);
}
static gboolean keyPressed(GtkWidget *view, GdkEventKey *event, gpointer data) {
    SinanGTKSurface *s = data;
    if (!ownDocument(s) || !gtk_widget_get_visible(view)) return FALSE;
    GdkEventKey normalized = *event;
    GdkKeymap *keymap = gdk_keymap_get_for_display(gtk_widget_get_display(view));
    GdkModifierType state = normalized.state;
    gdk_keymap_add_virtual_modifiers(keymap,&state);
    normalized.state = state;
    event = &normalized;
    guint mods = event->state & (GDK_CONTROL_MASK|GDK_MOD1_MASK|GDK_SHIFT_MASK|GDK_SUPER_MASK|GDK_META_MASK|GDK_HYPER_MASK|GDK_MOD3_MASK|GDK_MOD5_MASK);
    int action = 0;
    if (event->keyval == GDK_KEY_Escape && mods == 0) action = 3;
    else if (shortcutMatches("Ctrl+Comma",event)) action = 2;
    else if (shortcutMatches(s->shortcut,event)) action = 1;
    if (!action) return FALSE;
    sinanGTKAction(s->handle,action); return TRUE;
}

static void pingFinished(GObject *object, GAsyncResult *result, gpointer data) {
    WebKitWebView *view = WEBKIT_WEB_VIEW(object);
    SinanGTKSurface *s = g_object_get_data(object,SINAN_DATA);
    GError *error = NULL;
    JSCValue *value = webkit_web_view_evaluate_javascript_finish(view,result,&error);
    if (s) { s->pingPending = FALSE; s->lastPing = g_get_monotonic_time(); if (error) failSurface(s); }
    if (value) g_object_unref(value);
    g_clear_error(&error); g_object_unref(data);
}
static gboolean timer(gpointer data) {
    WebKitWebView *view = data;
    SinanGTKSurface *s = g_object_get_data(G_OBJECT(view),SINAN_DATA);
    if (!active(s)) return G_SOURCE_CONTINUE;
    if (!s->ready) return G_SOURCE_CONTINUE;
    gint64 now = g_get_monotonic_time();
    if (s->pingPending) { if (now-s->pingStarted > G_USEC_PER_SEC) failSurface(s); return G_SOURCE_CONTINUE; }
    if (now-s->lastPing < G_USEC_PER_SEC/4) return G_SOURCE_CONTINUE;
    s->pingPending = TRUE; s->pingStarted = now;
    webkit_web_view_evaluate_javascript(view,"true",-1,SINAN_WORLD,NULL,s->cancellable,pingFinished,g_object_ref(view));
    return G_SOURCE_CONTINUE;
}
static void addScript(WebKitUserContentManager *manager, const char *script, const char *world) {
    WebKitUserScript *value = webkit_user_script_new_for_world(script,WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START,world,NULL,NULL);
    webkit_user_content_manager_add_script(manager,value); webkit_user_script_unref(value);
}

SinanGTKSurface *sinanGTKCreate(void *window, unsigned long long handle, const char *entry, const char *csp, const char *pageScript, const char *isolatedScript, const char *themeScript, int bridgeVersion) {
    if (!window || !GTK_IS_WINDOW(window) || !handle || !entry) return NULL;
    GtkWidget *overlay = g_object_get_data(G_OBJECT(window),"sinan-plugin-overlay");
    if (!overlay) {
        GtkWidget *child = gtk_bin_get_child(GTK_BIN(window));
        if (!child) return NULL;
        g_object_ref(child); gtk_container_remove(GTK_CONTAINER(window),child);
        overlay = gtk_overlay_new(); gtk_container_add(GTK_CONTAINER(overlay),child); g_object_unref(child);
        gtk_container_add(GTK_CONTAINER(window),overlay);
        g_object_set_data(G_OBJECT(window),"sinan-plugin-overlay",overlay);
        gtk_widget_show(overlay);
    }
    WebKitWebContext *context = webkit_web_context_new_ephemeral();
    SchemeData *scheme = g_new0(SchemeData,1); scheme->handle=handle; scheme->csp=g_strdup(csp);
    webkit_web_context_register_uri_scheme(context,"sinan-plugin",schemeRequest,scheme,schemeFree);
    WebKitSecurityManager *security = webkit_web_context_get_security_manager(context);
    webkit_security_manager_register_uri_scheme_as_secure(security,"sinan-plugin");
    webkit_security_manager_register_uri_scheme_as_cors_enabled(security,"sinan-plugin");
    WebKitUserContentManager *ownManager = webkit_user_content_manager_new();
    WebKitWebView *view = g_object_new(WEBKIT_TYPE_WEB_VIEW,"web-context",context,"user-content-manager",ownManager,NULL);
    g_object_unref(ownManager);
    g_object_ref_sink(view);
    g_signal_connect(context,"download-started",G_CALLBACK(download),NULL);
    g_object_unref(context);
    SinanGTKSurface *s = g_new0(SinanGTKSurface,1);
    s->view=view; s->overlay=g_object_ref(overlay); s->entry=g_strdup(entry); s->handle=handle; s->bridgeVersion=bridgeVersion;
    s->cancellable=g_cancellable_new();
    g_object_set_data_full(G_OBJECT(view),SINAN_DATA,s,surfaceFree);
    WebKitSettings *settings = webkit_web_view_get_settings(view);
    webkit_settings_set_enable_developer_extras(settings,FALSE);
    webkit_settings_set_enable_write_console_messages_to_stdout(settings,FALSE);
    webkit_settings_set_enable_html5_database(settings,FALSE);
    webkit_settings_set_enable_html5_local_storage(settings,FALSE);
    webkit_settings_set_enable_dns_prefetching(settings,FALSE);
    webkit_settings_set_javascript_can_open_windows_automatically(settings,FALSE);
    webkit_settings_set_allow_file_access_from_file_urls(settings,FALSE);
    webkit_settings_set_allow_universal_access_from_file_urls(settings,FALSE);
    webkit_settings_set_enable_webrtc(settings,FALSE);
    webkit_settings_set_enable_media_stream(settings,FALSE);
    WebKitUserContentManager *manager = webkit_web_view_get_user_content_manager(view);
    if (!webkit_user_content_manager_register_script_message_handler_in_world(manager,"sinan",SINAN_WORLD) ||
        (bridgeVersion == 2 && !webkit_user_content_manager_register_script_message_handler_with_reply(manager,"sinanCall",SINAN_WORLD))) {
        sinanGTKClose(s); return NULL;
    }
    g_signal_connect(manager,"script-message-received::sinan",G_CALLBACK(message),s);
    g_signal_connect(manager,"script-message-with-reply-received::sinanCall",G_CALLBACK(call),s);
    addScript(manager,isolatedScript,SINAN_WORLD); addScript(manager,pageScript,NULL); addScript(manager,themeScript,NULL);
    g_signal_connect(view,"decide-policy",G_CALLBACK(navigation),s);
    g_signal_connect(view,"create",G_CALLBACK(newWindow),s);
    g_signal_connect(view,"permission-request",G_CALLBACK(permission),s);
    g_signal_connect(view,"run-file-chooser",G_CALLBACK(chooser),s);
    g_signal_connect(view,"script-dialog",G_CALLBACK(dialog),s);
    g_signal_connect(view,"context-menu",G_CALLBACK(contextMenu),s);
    g_signal_connect(view,"load-failed",G_CALLBACK(loadFailed),s);
    g_signal_connect(view,"web-process-terminated",G_CALLBACK(rendererFailed),s);
    g_signal_connect(view,"destroy",G_CALLBACK(viewDestroyed),s);
    g_signal_connect(view,"key-press-event",G_CALLBACK(keyPressed),s);
    gtk_widget_set_no_show_all(GTK_WIDGET(view),TRUE);
    gtk_widget_set_halign(GTK_WIDGET(view),GTK_ALIGN_FILL); gtk_widget_set_valign(GTK_WIDGET(view),GTK_ALIGN_START);
    gtk_widget_set_margin_top(GTK_WIDGET(view),58);
    gtk_overlay_add_overlay(GTK_OVERLAY(overlay),GTK_WIDGET(view));
    g_signal_connect(overlay,"size-allocate",G_CALLBACK(resized),s);
    GtkAllocation allocation; gtk_widget_get_allocation(overlay,&allocation); resized(overlay,&allocation,s);
    s->timer=g_timeout_add_full(G_PRIORITY_DEFAULT,100,timer,g_object_ref(view),g_object_unref);
    webkit_web_view_load_uri(view,entry);
    return s;
}
int sinanGTKStatus(SinanGTKSurface *s) { return !active(s) ? -1 : s->ready ? 1 : 0; }
void sinanGTKShow(SinanGTKSurface *s) { if (active(s)) { gtk_widget_show(GTK_WIDGET(s->view)); gtk_widget_grab_focus(GTK_WIDGET(s->view)); } }
void sinanGTKTheme(SinanGTKSurface *s, const char *script) {
    if (ownDocument(s)) webkit_web_view_evaluate_javascript(s->view,script,-1,NULL,NULL,s->cancellable,NULL,NULL);
}
void sinanGTKShortcut(SinanGTKSurface *s, const char *shortcut) { if (active(s)) { g_free(s->shortcut); s->shortcut=g_strdup(shortcut); } }
void sinanGTKDetached(SinanGTKSurface *s) {
    if (!active(s)) return;
    s->detached=TRUE; GtkAllocation allocation; gtk_widget_get_allocation(s->overlay,&allocation); resized(s->overlay,&allocation,s);
}
void sinanGTKClose(SinanGTKSurface *s) {
    if (!s || s->closed) return;
    s->closed=TRUE; s->handle=0;
    g_cancellable_cancel(s->cancellable);
    if (s->timer) { g_source_remove(s->timer); s->timer=0; }
    WebKitUserContentManager *manager=webkit_web_view_get_user_content_manager(s->view);
    webkit_user_content_manager_unregister_script_message_handler_in_world(manager,"sinan",SINAN_WORLD);
    webkit_user_content_manager_unregister_script_message_handler_in_world(manager,"sinanCall",SINAN_WORLD);
    g_signal_handlers_disconnect_by_data(manager,s); g_signal_handlers_disconnect_by_data(s->view,s); g_signal_handlers_disconnect_by_data(s->overlay,s);
    webkit_web_view_stop_loading(s->view);
    GtkWidget *overlay=s->overlay; WebKitWebView *view=s->view;
    gtk_widget_destroy(GTK_WIDGET(view)); g_object_unref(overlay); g_object_unref(view);
}
