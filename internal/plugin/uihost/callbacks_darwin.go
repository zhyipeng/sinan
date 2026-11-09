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
	"sinan/internal/plugin/uiprocess"
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
	NextControl() uiprocess.Control
	WindowAction(string) error
	Focused(uint64) error
}

//export sinanUIPollControl
func sinanUIPollControl(handle C.ulonglong, id *C.ulonglong) (result C.int) {
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
		control := client.NextControl()
		*id = C.ulonglong(control.ID)
		switch control.Kind {
		case "focus":
			return 1
		case "toggle":
			return 2
		case "detach":
			return 3
		}
	}
	return 0
}

//export sinanUIWindowAction
func sinanUIWindowAction(handle C.ulonglong, kind C.int) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return 0
	}
	if kind != 1 && kind != 2 {
		return 0
	}
	action := "detach"
	if kind == 2 {
		action = "back"
	}
	if client, ok := binding.session.(focusSession); ok && client.WindowAction(action) == nil {
		return 1
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

//export sinanUIDetachShortcut
func sinanUIDetachShortcut(handle C.ulonglong) (result *C.char) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return nil
	}
	if client, ok := binding.session.(interface{ DetachShortcut() string }); ok {
		return C.CString(client.DetachShortcut())
	}
	return nil
}

//export sinanUISessionCall
func sinanUISessionCall(handle C.ulonglong, data *C.char, length C.int) (result *C.char) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	if length < 0 || length > uiview.MaxCallBytes {
		return nil
	}
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return nil
	}
	caller, ok := binding.session.(interface {
		Call([]byte) (json.RawMessage, error)
	})
	if !ok {
		return nil
	}
	value, err := caller.Call(C.GoBytes(unsafe.Pointer(data), length))
	if err != nil {
		value, _ = json.Marshal(map[string]string{"error": err.Error()})
	} else {
		value, _ = json.Marshal(map[string]json.RawMessage{"result": value})
	}
	return C.CString(string(value))
}
