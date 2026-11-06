//go:build darwin && !ios && cgo

package uihost

/*
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"

import (
	"encoding/json"
	"runtime/cgo"
	"sinan/internal/plugin/uiview"
	"unsafe"
)

type nativeBinding struct {
	session Session
	disable func() error
}

func bindingFor(handle C.ulonglong) *nativeBinding {
	return cgo.Handle(handle).Value().(*nativeBinding)
}

//export sinanUISessionResource
func sinanUISessionResource(handle C.ulonglong, url *C.char) (result *C.char) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	resource, ok := bindingFor(handle).session.Resource(C.GoString(url))
	if !ok {
		return nil
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

//export sinanUISessionMessage
func sinanUISessionMessage(handle C.ulonglong, data *C.char, length C.int) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	if length < 0 || length > uiview.MaxMessageBytes {
		return 0
	}
	action, err := bindingFor(handle).session.Message(C.GoBytes(unsafe.Pointer(data), length))
	if err != nil {
		return 0
	}
	return C.int(action)
}

//export sinanUISessionRevoke
func sinanUISessionRevoke(handle C.ulonglong) {
	defer func() { _ = recover() }()
	bindingFor(handle).session.Revoke()
}

//export sinanUIProbeDisable
func sinanUIProbeDisable(handle C.ulonglong) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	binding := bindingFor(handle)
	if binding.disable != nil && binding.session.Active() && binding.disable() == nil {
		return 1
	}
	return 0
}

type focusSession interface {
	NextFocus() uint64
	Focused(uint64) error
}

//export sinanUIPollFocus
func sinanUIPollFocus(handle C.ulonglong) (result C.ulonglong) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return 0
	}
	if client, ok := binding.session.(focusSession); ok {
		return C.ulonglong(client.NextFocus())
	}
	return 0
}

//export sinanUIFocused
func sinanUIFocused(handle C.ulonglong, id C.ulonglong) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return 0
	}
	if client, ok := binding.session.(focusSession); ok && client.Focused(uint64(id)) == nil {
		return 1
	}
	return 0
}
