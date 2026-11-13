//go:build windows && (amd64 || arm64) && !native_webview2loader

package uihost

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	themesdk "sinan/packages/plugin-sdk/theme"
)

//go:embed webview_page_bridge.js
var webviewPageBridge string

const EmbeddedSupported = true

var webviewUser32 = windows.NewLazySystemDLL("user32.dll")
var webviewGetClientRect = webviewUser32.NewProc("GetClientRect")
var webviewGetDPI = webviewUser32.NewProc("GetDpiForWindow")
var webviewGetKeyState = webviewUser32.NewProc("GetKeyState")
var webviewMemStream = windows.NewLazySystemDLL("ole32.dll").NewProc("CreateStreamOnHGlobal")

type webviewSubscription struct {
	owner  *webviewCOM
	remove int
	token  int64
}

type Embedded struct {
	session   *uiview.Session
	transport *webviewTransport
	invoke    func(func())
	action    func(int)
	done      chan struct{}
	closed    atomic.Bool
	// Everything below is accessed only on the Wails UI thread.
	hwnd                                           uintptr
	env, controller, web                           *webviewCOM
	subscriptions                                  []webviewSubscription
	callbacks                                      []*webviewCallback
	folder                                         string
	script                                         string
	status                                         int
	shown, detached, callPending, heartbeatPending bool
	themeRevision, shortcutRevision                uint64
	shortcut                                       string
	heartbeat                                      time.Time
}

type webviewEnvironmentHandler struct{ e *Embedded }

func (h *webviewEnvironmentHandler) EnvironmentCompleted(code webviewloader.HRESULT, env *webviewloader.ICoreWebView2Environment) webviewloader.HRESULT {
	e := h.e
	if e.closed.Load() {
		return 0
	}
	if code < 0 || env == nil {
		e.fail()
		return 0
	}
	// The loader releases its borrowed environment after this callback. QI gives
	// us an owned reference and fails closed on runtimes without private profiles.
	var err error
	e.env, err = (*webviewCOM)(unsafe.Pointer(env)).query("ee0eb9df-6f12-46ce-b53f-3f47b9c928e0")
	if err != nil {
		e.fail()
		return 0
	}
	options, err := e.env.object(20)
	if err != nil {
		e.fail()
		return 0
	}
	defer options.release()
	if options.call(4, uintptr(unsafe.Pointer(webviewString("sinan")))) != nil || options.call(6, 1) != nil {
		e.fail()
		return 0
	}
	private, err := options.number(5)
	if err != nil || private != 1 {
		e.fail()
		return 0
	}
	callback := e.callback("6c4819f3-c9b7-4260-8127-c9f5bde7f68c", func(code uintptr, result unsafe.Pointer) uintptr {
		controller := (*webviewCOM)(result)
		if e.closed.Load() {
			if controller != nil {
				_ = controller.call(24)
			}
			return 0
		}
		if int32(code) < 0 || controller == nil {
			e.fail()
			return 0
		}
		_ = controller.call(1)
		e.controller = controller
		if e.setup() != nil {
			e.fail()
		}
		return 0
	})
	if e.env.call(21, e.hwnd, uintptr(unsafe.Pointer(options)), callback.pointer()) != nil {
		e.fail()
	}
	return 0
}

func PrepareEmbedded(ctx context.Context, session *uiview.Session, window func() unsafe.Pointer, state protocol.ThemeState, shortcut string, action func(int), invoke func(func())) (*Embedded, error) {
	if session == nil || window == nil || invoke == nil || theme.ValidateState(state) != nil {
		return nil, errors.New("插件视图不可用")
	}
	bridge, err := newNativeBridge(session, session.EntryURL())
	if err != nil {
		return nil, err
	}
	folder, err := os.MkdirTemp("", "sinan-webview2-")
	if err != nil {
		return nil, errors.New("无法创建插件临时配置")
	}
	e := &Embedded{session: session, transport: newWebviewTransport(bridge), invoke: invoke, action: action, done: make(chan struct{}), folder: folder, themeRevision: state.Revision, shortcut: shortcut}
	entryJSON, _ := json.Marshal(e.transport.entry)
	stateJSON, _ := json.Marshal(state)
	e.script = strings.ReplaceAll(webviewPageBridge, "__SINAN_ENTRY__", string(entryJSON)) + ";" + themesdk.Runtime + ";window.__sinanApplyTheme(" + string(stateJSON) + ");"
	invoke(func() {
		e.hwnd = uintptr(window())
		if e.hwnd == 0 || !session.Active() {
			e.fail()
			return
		}
		err := webviewloader.CreateCoreWebView2EnvironmentWithOptions(&webviewEnvironmentHandler{e}, webviewloader.WithUserDataFolder(folder), webviewloader.WithExclusiveUserDataFolderAccess(true), webviewloader.WithAllowSingleSignOnUsingOSPrimaryAccount(false), webviewloader.WithTargetCompatibleBrowserVersion("119.0.0.0"))
		if err != nil {
			e.fail()
		}
	})
	unsubscribe := session.OnRevoke(func() { go e.Close() })
	go func() { <-e.done; unsubscribe() }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := 0
		invoke(func() { status = e.status })
		if status < 0 {
			e.Close()
			return nil, errors.New("无法准备隔离 WebView2，请检查运行时版本")
		}
		if status == 1 && session.Active() {
			go e.monitor()
			return e, nil
		}
		select {
		case <-ctx.Done():
			e.Close()
			return nil, errors.New("插件页面加载超时或已取消")
		case <-e.done:
			return nil, errors.New("插件页面已关闭")
		case <-ticker.C:
		}
	}
}

func (e *Embedded) callback(id string, run func(uintptr, unsafe.Pointer) uintptr) *webviewCallback {
	c := newWebviewCallback(id, run)
	e.callbacks = append(e.callbacks, c)
	return c
}
func (e *Embedded) event(owner *webviewCOM, add, remove int, id string, run func(*webviewCOM) error) error {
	callback := e.callback(id, func(_ uintptr, arg unsafe.Pointer) uintptr {
		defer func() {
			if recover() != nil {
				e.fail()
			}
		}()
		if e.closed.Load() {
			return 0
		}
		if err := run((*webviewCOM)(arg)); err != nil {
			e.fail()
		}
		return 0
	})
	var token int64
	if err := owner.call(add, callback.pointer(), uintptr(unsafe.Pointer(&token))); err != nil {
		return err
	}
	// Hold an owner reference until its handler has been removed.
	_ = owner.call(1)
	e.subscriptions = append(e.subscriptions, webviewSubscription{owner, remove, token})
	return nil
}
func (e *Embedded) setup() error {
	if err := e.controller.call(4, 0); err != nil {
		return err
	}
	var err error
	e.web, err = e.controller.object(25)
	if err != nil {
		return err
	}
	settings, err := e.web.object(3)
	if err != nil {
		return err
	}
	defer settings.release()
	for _, slot := range []int{8, 10, 12, 14, 16, 18, 20} {
		if err = settings.call(slot, 0); err != nil {
			return err
		}
	}
	settings3, err := settings.query("fdb5ab74-af33-4854-84f0-0a631deb5eba")
	if err != nil {
		return err
	}
	defer settings3.release()
	if err = settings3.call(24, 0); err != nil {
		return err
	}
	if err = e.event(e.web, 7, 8, "9adbe429-f36d-432b-9ddc-f8881fbd76e3", func(args *webviewCOM) error {
		uri, err := args.text(3, 1024)
		redirect, redirectErr := args.number(5)
		if err != nil || redirectErr != nil || redirect != 0 || !e.transport.navigate(uri, true) {
			return args.call(8, 1)
		}
		return nil
	}); err != nil {
		return err
	}
	if err = e.event(e.web, 17, 18, "9adbe429-f36d-432b-9ddc-f8881fbd76e3", func(args *webviewCOM) error { return args.call(8, 1) }); err != nil {
		return err
	}
	if err = e.event(e.web, 15, 16, "d33a35bf-1c49-4f98-93ab-006e0533fe1c", func(args *webviewCOM) error {
		success, err := args.number(3)
		if err != nil || success == 0 {
			return errors.New("导航失败")
		}
		return nil
	}); err != nil {
		return err
	}
	if err = e.event(e.web, 23, 24, "15e1c6a3-c72a-4df3-91d7-d097fbec6bfd", func(args *webviewCOM) error { return args.call(7, 2) }); err != nil {
		return err
	}
	if err = e.event(e.web, 44, 45, "d4c185fe-c81c-4989-97af-2d3fa7ab5651", func(args *webviewCOM) error { return args.call(6, 1) }); err != nil {
		return err
	}
	// No Accept call: with default dialogs disabled, this leaves dialogs canceled.
	if err = e.event(e.web, 21, 22, "ef381bf9-afa8-4e37-91c4-8ac48524bdfb", func(*webviewCOM) error { return nil }); err != nil {
		return err
	}
	if err = e.event(e.web, 25, 26, "79e0aea4-990b-42d9-aa1d-0fcc2e5bc7f1", func(*webviewCOM) error { return errors.New("渲染进程已终止") }); err != nil {
		return err
	}
	if err = e.event(e.web, 59, 60, "5c19e9e0-092f-486b-affa-ca8231913039", func(*webviewCOM) error { go e.Close(); return nil }); err != nil {
		return err
	}
	if err = e.event(e.web, 55, 56, "ab00b74c-15f1-4646-80e8-e76341d25d71", e.resource); err != nil {
		return err
	}
	if err = e.web.call(57, uintptr(unsafe.Pointer(webviewString("*"))), 0); err != nil {
		return err
	}
	if err = e.event(e.web, 34, 35, "57213f19-00e6-49fa-8e07-898ea01ecbd2", e.message); err != nil {
		return err
	}
	web4, err := e.web.query("20d02d59-6df2-42dc-bd06-f98a694b1302")
	if err != nil {
		return err
	}
	defer web4.release()
	if err = e.event(web4, 75, 76, "efedc989-c396-41ca-83f7-07f845a55724", func(args *webviewCOM) error {
		if err := args.call(5, 1); err != nil {
			return err
		}
		return args.call(9, 1)
	}); err != nil {
		return err
	}
	if err = e.event(e.controller, 19, 20, "b29c7e28-fa79-41a8-8e44-65811c76dcb2", e.key); err != nil {
		return err
	}
	if err = e.resize(); err != nil {
		return err
	}
	// Both asynchronous policy operations must complete before the first load.
	// CDP stays host-private; it is never exposed through the page's narrow bridge.
	cdp := e.callback("5c4889f0-5ef6-4c5a-952c-d8f1b92d0574", func(code uintptr, _ unsafe.Pointer) uintptr {
		if e.closed.Load() {
			return 0
		}
		if int32(code) < 0 {
			e.fail()
			return 0
		}
		completion := e.callback("b99369f3-9b11-47b5-bc6f-8e7895fcea17", func(code uintptr, _ unsafe.Pointer) uintptr {
			if !e.closed.Load() {
				if int32(code) < 0 || e.web.call(5, uintptr(unsafe.Pointer(webviewString(e.transport.entry)))) != nil {
					e.fail()
				}
			}
			return 0
		})
		if e.web.call(27, uintptr(unsafe.Pointer(webviewString(e.script))), completion.pointer()) != nil {
			e.fail()
		}
		return 0
	})
	return e.web.call(36, uintptr(unsafe.Pointer(webviewString("Page.setInterceptFileChooserDialog"))), uintptr(unsafe.Pointer(webviewString(`{"enabled":true}`))), cdp.pointer())
}

func (e *Embedded) resource(args *webviewCOM) error {
	// Install a deny response before reading page-controlled request metadata.
	// Later stream/allocation/parser failures retain it, even before teardown.
	if err := e.resourceResponse(args, 404, "Not Found", "text/plain", nil); err != nil {
		return err
	}
	request, err := args.object(3)
	if err != nil {
		return err
	}
	defer request.release()
	uri, uriErr := request.text(3, 1024)
	method, methodErr := request.text(5, 16)
	if uriErr != nil || methodErr != nil {
		e.transport.bridge.consume(0)
		return nil
	}
	resource, ok := e.transport.resource(method, uri)
	if !ok {
		return nil
	}
	return e.resourceResponse(args, 200, "OK", resource.MIME, resource.Data)
}
func (e *Embedded) resourceResponse(args *webviewCOM, status uintptr, reason, mime string, data []byte) error {
	// An owned HGLOBAL stream copies the snapshot; no Go buffer is retained.
	var stream *webviewCOM
	if len(data) > 0 {
		result, _, _ := webviewMemStream.Call(0, 1, uintptr(unsafe.Pointer(&stream)))
		if int32(result) < 0 || stream == nil {
			return errors.New("无法创建插件资源流")
		}
		defer stream.release()
		var written uint32
		if err := stream.call(4, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written))); err != nil || int(written) != len(data) {
			return errors.New("无法写入插件资源流")
		}
		var position uint64
		if err := stream.call(5, 0, 0, uintptr(unsafe.Pointer(&position))); err != nil {
			return err
		}
	}

	if strings.ContainsAny(mime, "\r\n") {
		return errors.New("插件资源类型无效")
	}
	headers := "Content-Type: " + mime + "\r\nContent-Security-Policy: " + e.transport.csp() + "\r\nCache-Control: no-store\r\nX-Content-Type-Options: nosniff\r\n"
	var response *webviewCOM
	if err := e.env.call(4, uintptr(unsafe.Pointer(stream)), status, uintptr(unsafe.Pointer(webviewString(reason))), uintptr(unsafe.Pointer(webviewString(headers))), uintptr(unsafe.Pointer(&response))); err != nil {
		return err
	}
	if response == nil {
		return errors.New("无法创建插件资源响应")
	}
	defer response.release()
	return args.call(5, uintptr(unsafe.Pointer(response)))
}
func (e *Embedded) message(args *webviewCOM) error {
	source, err := args.text(3, 1024)
	if err != nil {
		return err
	}
	data, err := args.text(4, webviewMaxEnvelope)
	if err != nil {
		e.transport.bridge.consume(0)
		return nil
	}
	envelope, ok := e.transport.receive(source, []byte(data))
	if !ok {
		return nil
	}
	// This is the top-level CoreWebView2 event, not a frame's event. No provenance
	// is inferred from JavaScript-supplied origin/frame fields.
	if envelope.Kind == "message" {
		action, err := e.transport.bridge.Message(e.session.EntryURL(), true, envelope.Request)
		if err != nil {
			return nil
		}
		if action == uiview.Ready {
			e.status = 1
			e.heartbeat = time.Now()
		}
		if action == uiview.Close {
			go e.Close()
		}
		return nil
	}
	if e.callPending {
		e.transport.bridge.consume(0)
		e.reply(envelope.ID, nil, true)
		return nil
	}
	e.callPending = true
	go func() {
		value, err := e.transport.bridge.Call(e.session.EntryURL(), true, envelope.Request)
		if err == nil {
			value, err = e.transport.callResult(envelope.Request, value)
		}
		e.invoke(func() {
			e.callPending = false
			if !e.closed.Load() && e.session.Active() {
				e.reply(envelope.ID, value, err != nil)
			}
		})
	}()
	return nil
}
func (e *Embedded) reply(id uint64, value json.RawMessage, failed bool) {
	if e.closed.Load() || !e.session.Active() {
		return
	}
	reply := struct {
		Channel string          `json:"channel"`
		ID      uint64          `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   string          `json:"error,omitempty"`
	}{Channel: "sinan:reply", ID: id, Result: value}
	if failed {
		reply.Result = nil
		reply.Error = "UI 调用失败或会话已关闭"
	}
	data, err := json.Marshal(reply)
	if err != nil || e.web.call(32, uintptr(unsafe.Pointer(webviewString(string(data))))) != nil {
		e.fail()
	}
}
func (e *Embedded) key(args *webviewCOM) error {
	if !e.shown || !e.session.Active() {
		return nil
	}
	kind, err := args.number(3)
	if err != nil {
		return err
	}
	if kind != 0 && kind != 2 {
		return nil
	}
	vk, err := args.number(4)
	if err != nil {
		return err
	}
	flags, err := args.number(5)
	if err != nil {
		return err
	}
	var mods uint32
	for _, key := range []struct {
		vk  uintptr
		mod uint32
	}{{0x11, webviewCtrl}, {0x10, webviewShift}, {0x12, webviewAlt}, {0x5b, webviewMeta}, {0x5c, webviewMeta}} {
		state, _, _ := webviewGetKeyState.Call(key.vk)
		if state&0x8000 != 0 {
			mods |= key.mod
		}
	}
	rightAlt, _, _ := webviewGetKeyState.Call(0xa5)
	if rightAlt&0x8000 != 0 && mods&(webviewCtrl|webviewAlt) == webviewCtrl|webviewAlt {
		mods |= webviewAltGraph
	}
	action := webviewShortcutAction(e.shortcut, vk, (flags>>16)&0xff, mods, flags&(1<<30) != 0)
	if action == 0 {
		return nil
	}
	if err := args.call(8, 1); err != nil {
		return err
	}
	if e.action != nil {
		go func() {
			if e.session.Active() {
				e.action(action)
			}
		}()
	}
	return nil
}
func (e *Embedded) resize() error {
	var rect webviewRect
	result, _, _ := webviewGetClientRect.Call(e.hwnd, uintptr(unsafe.Pointer(&rect)))
	if result == 0 {
		return errors.New("插件父窗口不可用")
	}
	dpi := uint32(96)
	if webviewGetDPI.Find() == nil {
		value, _, _ := webviewGetDPI.Call(e.hwnd)
		dpi = uint32(value)
	}
	bounds := webviewBodyBounds(rect.Right, rect.Bottom, dpi, e.detached)
	if err := setWebviewBounds(e.controller, &bounds); err != nil {
		return err
	}
	return e.controller.call(23)
}
func (e *Embedded) fail() {
	e.status = -1
	// Close synchronously before returning a resource failure, so an unset
	// Response can never fall through to network handling.
	if e.web != nil {
		_ = e.web.call(43)
	}
	if e.controller != nil {
		_ = e.controller.call(24)
	}
	e.session.Revoke()
	go e.Close()
}
func (e *Embedded) monitor() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Hour)
	defer deadline.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-deadline.C:
			e.Close()
			return
		case <-ticker.C:
			e.invoke(func() {
				if e.closed.Load() {
					return
				}
				if e.resize() != nil || time.Since(e.heartbeat) > time.Second {
					e.fail()
					return
				}
				if e.heartbeatPending {
					return
				}
				e.heartbeatPending = true
				callback := newWebviewCallback("49511172-cc67-4bca-9923-137112f4c4cc", func(code uintptr, result unsafe.Pointer) uintptr {
					e.heartbeatPending = false
					if e.closed.Load() {
						return 0
					}
					text, err := webviewUTF16Text((*uint16)(result), 16)
					if int32(code) < 0 || err != nil || text != "true" {
						e.fail()
					} else {
						e.heartbeat = time.Now()
					}
					return 0
				})
				// The async API owns its own reference until completion; do not accumulate
				// a callback in e.callbacks on every heartbeat.
				err := e.web.call(29, uintptr(unsafe.Pointer(webviewString("true"))), callback.pointer())
				callback.release()
				if err != nil {
					e.fail()
				}
			})
		}
	}
}
func (e *Embedded) Done() <-chan struct{} { return e.done }
func (e *Embedded) Show() {
	e.invoke(func() {
		if !e.closed.Load() && e.status == 1 {
			e.shown = true
			if e.resize() != nil || e.controller.call(4, 1) != nil || e.controller.call(12, 0) != nil {
				e.fail()
			}
		}
	})
}
func (e *Embedded) Close() {
	if !e.closed.CompareAndSwap(false, true) {
		return
	}
	e.session.Revoke()
	e.invoke(func() {
		for _, s := range e.subscriptions {
			_ = s.owner.call(s.remove, uintptr(s.token))
			s.owner.release()
		}
		e.subscriptions = nil
		if e.controller != nil {
			_ = e.controller.call(24)
		}
		e.web.release()
		e.controller.release()
		e.env.release()
		e.web = nil
		e.controller = nil
		e.env = nil
		for _, callback := range e.callbacks {
			callback.release()
		}
		e.callbacks = nil
		close(e.done)
	})
	// WebView2 may retain files briefly after Close. This folder is never reused.
	// Crash leftovers are possible; InPrivate is not a promise of diskless storage.
	go func() {
		for range 40 {
			if os.RemoveAll(e.folder) == nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
	}()
}
func (e *Embedded) SetTheme(state protocol.ThemeState) {
	if theme.ValidateState(state) != nil {
		return
	}
	data, _ := json.Marshal(state)
	e.invoke(func() {
		if !e.closed.Load() && state.Revision >= e.themeRevision {
			e.themeRevision = state.Revision
			if e.web.call(29, uintptr(unsafe.Pointer(webviewString("window.__sinanApplyTheme("+string(data)+");"))), 0) != nil {
				e.fail()
			}
		}
	})
}
func (e *Embedded) SetDetachShortcut(shortcut string, revision uint64) {
	e.invoke(func() {
		if !e.closed.Load() && revision >= e.shortcutRevision {
			e.shortcutRevision = revision
			e.shortcut = shortcut
		}
	})
}
func (e *Embedded) SetDetached() {
	e.invoke(func() {
		if !e.closed.Load() {
			e.detached = true
			if e.resize() != nil {
				e.fail()
			}
		}
	})
}
