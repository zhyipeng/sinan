//go:build linux && cgo && gtk3

package uihost

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <stdlib.h>
#include "native_linux.h"
*/
import "C"

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"runtime/cgo"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	themesdk "sinan/packages/plugin-sdk/theme"
)

//go:embed gtk_page_bridge.js
var gtkPageBridge string

//go:embed gtk_isolated_bridge.js
var gtkIsolatedBridge string

const EmbeddedSupported = true

type gtkBinding struct {
	bridge *nativeBridge
	invoke func(func())
	action func(int)
}

type Embedded struct {
	pointer          *C.SinanGTKSurface
	handle           cgo.Handle
	session          *uiview.Session
	invoke           func(func())
	done             chan struct{}
	closed           atomic.Bool
	themeRevision    uint64
	shortcutRevision uint64
}

func PrepareEmbedded(ctx context.Context, session *uiview.Session, window func() unsafe.Pointer, state protocol.ThemeState, shortcut string, action func(int), invoke func(func())) (*Embedded, error) {
	if session == nil || window == nil || invoke == nil || theme.ValidateState(state) != nil {
		return nil, errors.New("插件视图不可用")
	}
	bridge, err := newNativeBridge(session, session.EntryURL())
	if err != nil {
		return nil, err
	}
	entryJSON, _ := json.Marshal(session.EntryURL())
	stateJSON, _ := json.Marshal(state)
	values := []string{session.EntryURL(), uiview.CSP, strings.ReplaceAll(gtkPageBridge, "__SINAN_ENTRY__", string(entryJSON)), strings.ReplaceAll(gtkIsolatedBridge, "__SINAN_ENTRY__", string(entryJSON)), themesdk.Runtime + ";window.__sinanApplyTheme(" + string(stateJSON) + ");"}
	var args []*C.char
	for _, value := range values {
		encoded := C.CString(value)
		args = append(args, encoded)
		defer C.free(unsafe.Pointer(encoded))
	}
	e := &Embedded{session: session, invoke: invoke, done: make(chan struct{}), themeRevision: state.Revision}
	e.handle = cgo.NewHandle(&gtkBinding{bridge: bridge, invoke: invoke, action: action})
	version := C.int(1)
	if session.Binding().BridgeVersion == "2" {
		version = 2
	}
	invoke(func() {
		if native := window(); native != nil {
			e.pointer = C.sinanGTKCreate(native, C.ulonglong(e.handle), args[0], args[1], args[2], args[3], args[4], version)
		}
	})
	if e.pointer == nil {
		e.handle.Delete()
		return nil, errors.New("无法创建 GTK 隔离插件内容区")
	}
	unsubscribe := session.OnRevoke(func() { go e.Close() })
	go func() { <-e.done; unsubscribe() }()
	e.SetDetachShortcut(shortcut, 0)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		switch e.status() {
		case 1:
			go e.monitor()
			return e, nil
		case -1:
			e.Close()
			return nil, errors.New("插件页面加载失败")
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

func (e *Embedded) status() int {
	status := -1
	e.invoke(func() {
		if e.pointer != nil {
			status = int(C.sinanGTKStatus(e.pointer))
		}
	})
	return status
}
func (e *Embedded) monitor() {
	ticker := time.NewTicker(200 * time.Millisecond)
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
			if e.status() < 0 {
				e.Close()
				return
			}
		}
	}
}
func (e *Embedded) Done() <-chan struct{} { return e.done }
func (e *Embedded) Show() {
	e.invoke(func() {
		if e.pointer != nil {
			C.sinanGTKShow(e.pointer)
		}
	})
}
func (e *Embedded) Close() {
	if !e.closed.CompareAndSwap(false, true) {
		return
	}
	e.session.Revoke()
	e.invoke(func() {
		if e.pointer != nil {
			C.sinanGTKClose(e.pointer)
			e.pointer = nil
		}
		e.handle.Delete()
		close(e.done)
	})
}
func (e *Embedded) SetTheme(state protocol.ThemeState) {
	if theme.ValidateState(state) != nil {
		return
	}
	data, _ := json.Marshal(state)
	script := C.CString("window.__sinanApplyTheme(" + string(data) + ");")
	defer C.free(unsafe.Pointer(script))
	e.invoke(func() {
		if e.pointer != nil && state.Revision >= e.themeRevision {
			e.themeRevision = state.Revision
			C.sinanGTKTheme(e.pointer, script)
		}
	})
}
func (e *Embedded) SetDetachShortcut(shortcut string, revision uint64) {
	value := C.CString(shortcut)
	defer C.free(unsafe.Pointer(value))
	e.invoke(func() {
		if e.pointer != nil && revision >= e.shortcutRevision {
			e.shortcutRevision = revision
			C.sinanGTKShortcut(e.pointer, value)
		}
	})
}
func (e *Embedded) SetDetached() {
	e.invoke(func() {
		if e.pointer != nil {
			C.sinanGTKDetached(e.pointer)
		}
	})
}

// Deterministic native helpers; neither initializes GTK nor opens a display.
func gtkBodyHeight(height int, detached bool) int {
	var value C.int
	if detached {
		value = 1
	}
	return int(C.sinanGTKBodyHeight(C.int(height), value))
}
func matchesGTKDetachKey(shortcut, key string, modifiers uint32, hardware ...uint32) bool {
	var code uint32
	if len(hardware) > 0 {
		code = hardware[0]
	}
	value, name := C.CString(shortcut), C.CString(key)
	defer C.free(unsafe.Pointer(value))
	defer C.free(unsafe.Pointer(name))
	return C.sinanGTKShortcutMatches(value, name, C.uint(modifiers), C.uint(code)) != 0
}
