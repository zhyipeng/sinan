//go:build darwin && !ios && cgo

package uihost

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"unsafe"
)

const Supported = true

func runNative(data []byte, target string, session Session, disable func() error) (Report, error) {
	if !session.Active() {
		return Report{}, errors.New("UI 会话已撤销")
	}
	control := C.sinanUIControlNew()
	if control == nil {
		return Report{}, errors.New("无法创建 UI 撤销信号")
	}
	handle := cgo.NewHandle(&nativeBinding{session: session, disable: disable})
	var gateMu sync.Mutex
	live := true
	unsubscribe := session.OnRevoke(func() {
		gateMu.Lock()
		defer gateMu.Unlock()
		if live {
			C.sinanUIControlRevoke(control)
		}
	})
	defer func() {
		gateMu.Lock()
		live = false
		unsubscribe()
		C.sinanUIControlFree(control)
		gateMu.Unlock()
		handle.Delete()
	}()
	bundle := C.CString(string(data))
	defer C.free(unsafe.Pointer(bundle))
	url := C.CString(target)
	defer C.free(unsafe.Pointer(url))
	r := C.sinanUIProbe(bundle, url, C.ulonglong(handle), control)
	failure, known := map[C.int]string{C.SinanUIFailureNone: "", C.SinanUIFailureScript: "script-error", C.SinanUIFailureRenderer: "renderer-terminated", C.SinanUIFailureNavigation: "navigation-failed", C.SinanUIFailureUnresponsive: "unresponsive", C.SinanUIFailureDeadline: "deadline"}[r.failure]
	if !known {
		failure = "unknown"
	}
	return Report{Detached: r.detached != 0, TogglesHandled: int(r.togglesHandled), FocusHandled: int(r.focusHandled), EventLoopResponsive: r.eventLoopResponsive != 0, Failure: failure, HeartbeatCompleted: r.heartbeatCompleted != 0, Ready: r.ready != 0, BindingsAbsent: r.bindingsAbsent != 0, StorageIsolated: r.storageIsolated != 0, OwnResourceLoaded: r.ownResourceLoaded != 0, ForeignResourceBlocked: r.foreignResourceBlocked != 0, NavigationBlocked: r.navigationBlocked != 0, NetworkBlocked: r.networkBlocked != 0, InvalidMessageRejected: r.invalidMessageRejected != 0, CloseReceived: r.closeReceived != 0, BudgetRevoked: r.budgetRevoked != 0, LifecycleRevoked: r.lifecycleRevoked != 0}, nil
}

// Pure native key matching also supports deterministic regression without a window.
func matchesNativeDetachKey(shortcut string, code uint16, modifiers uint64) bool {
	value := C.CString(shortcut)
	defer C.free(unsafe.Pointer(value))
	return C.sinanUIDetachKeyMatches(value, C.ushort(code), C.ulong(modifiers)) != 0
}
