//go:build darwin && !ios && cgo

package main

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

func runNative(data []byte, target string, session viewSession, disable func() error) (report, error) {
	if !session.Active() {
		return report{}, errors.New("UI 会话已撤销")
	}
	control := C.sinanUIControlNew()
	if control == nil {
		return report{}, errors.New("无法创建 UI 撤销信号")
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
	return report{Ready: r.ready != 0, BindingsAbsent: r.bindingsAbsent != 0, StorageIsolated: r.storageIsolated != 0, OwnResourceLoaded: r.ownResourceLoaded != 0, ForeignResourceBlocked: r.foreignResourceBlocked != 0, NavigationBlocked: r.navigationBlocked != 0, NetworkBlocked: r.networkBlocked != 0, InvalidMessageRejected: r.invalidMessageRejected != 0, CloseReceived: r.closeReceived != 0, BudgetRevoked: r.budgetRevoked != 0, LifecycleRevoked: r.lifecycleRevoked != 0}, nil
}
