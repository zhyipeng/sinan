//go:build darwin && !ios && cgo

package uihost

/*
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/cgo"
	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	themesdk "sinan/packages/plugin-sdk/theme"
	"sync/atomic"
	"time"
	"unsafe"
)

type Embedded struct {
	themeRevision    uint64
	shortcutRevision uint64
	pointer          unsafe.Pointer
	handle           cgo.Handle
	session          *uiview.Session
	invoke           func(func())
	done             chan struct{}
	closed           atomic.Bool
}

// PrepareEmbedded loads a hidden isolated view in an existing window. The caller
// commits it with Show only after ready; failure leaves the previous page intact.
func PrepareEmbedded(ctx context.Context, session *uiview.Session, window func() unsafe.Pointer, state protocol.ThemeState, shortcut string, action func(int), invoke func(func())) (*Embedded, error) {
	if !session.Active() || window == nil || theme.ValidateState(state) != nil {
		return nil, errors.New("插件视图不可用")
	}
	binding := session.Binding()
	data, _ := json.Marshal(map[string]any{"entry": session.EntryURL(), "csp": uiview.CSP, "bridgeVersion": binding.BridgeVersion, "theme": state, "themeRuntime": themesdk.Runtime, "detachShortcut": shortcut})
	value := C.CString(string(data))
	defer C.free(unsafe.Pointer(value))
	e := &Embedded{session: session, invoke: invoke, done: make(chan struct{}), themeRevision: state.Revision}
	e.handle = cgo.NewHandle(&nativeBinding{session: &embeddedSession{Session: session}, embeddedAction: action})
	invoke(func() {
		if native := window(); native != nil {
			e.pointer = C.sinanUIEmbeddedCreate(value, C.ulonglong(e.handle), native)
		}
	})
	if e.pointer == nil {
		e.handle.Delete()
		return nil, errors.New("无法创建主窗口插件内容区")
	}
	unsubscribe := session.OnRevoke(func() { go e.Close() })
	go func() { <-e.done; unsubscribe() }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := e.status()
		if status == 1 {
			go e.monitor()
			return e, nil
		}
		if status < 0 {
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
			status = int(C.sinanUIEmbeddedStatus(e.pointer))
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
			C.sinanUIEmbeddedShow(e.pointer)
		}
	})
}
func (e *Embedded) SetTheme(state protocol.ThemeState) {
	if theme.ValidateState(state) != nil {
		return
	}
	data, _ := json.Marshal(state)
	value := C.CString(string(data))
	defer C.free(unsafe.Pointer(value))
	e.invoke(func() {
		if e.pointer != nil && state.Revision >= e.themeRevision {
			e.themeRevision = state.Revision
			C.sinanUIEmbeddedTheme(e.pointer, value)
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
			C.sinanUIEmbeddedClose(e.pointer)
			e.pointer = nil
		}
		e.handle.Delete()
		close(e.done)
	})
}

func (e *Embedded) SetDetachShortcut(shortcut string, revision uint64) {
	value := C.CString(shortcut)
	defer C.free(unsafe.Pointer(value))
	e.invoke(func() {
		if e.pointer != nil && revision >= e.shortcutRevision {
			e.shortcutRevision = revision
			C.sinanUIEmbeddedShortcut(e.pointer, value)
		}
	})
}

func (e *Embedded) SetDetached() {
	e.invoke(func() {
		if e.pointer != nil {
			C.sinanUIEmbeddedDetached(e.pointer)
		}
	})
}

func embeddedContentRect(width, height float64, detached, flipped bool) [4]float64 {
	var d, f C.int
	if detached {
		d = 1
	}
	if flipped {
		f = 1
	}
	value := C.sinanUIEmbeddedRect(C.double(width), C.double(height), d, f)
	return [4]float64{float64(value.x), float64(value.y), float64(value.width), float64(value.height)}
}
