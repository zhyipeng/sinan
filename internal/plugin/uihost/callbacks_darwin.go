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
	protocol "sinan/packages/plugin-protocol"
	"unsafe"
)

type nativeBinding struct {
	bridge         *nativeBridge
	embeddedAction func(int)
	session        Session
	disable        func() error
}

func bindingFor(handle C.ulonglong) *nativeBinding {
	return cgo.Handle(handle).Value().(*nativeBinding)
}

//export sinanUISessionResource
func sinanUISessionResource(handle C.ulonglong, method, url *C.char) (result *C.char) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	resource, ok := bindingFor(handle).bridge.Resource(C.GoString(method), C.GoString(url))
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
func sinanUISessionMessage(handle C.ulonglong, source *C.char, mainFrame C.int, data *C.char, length C.int) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	if length < 0 || length > uiview.MaxMessageBytes {
		return 0
	}
	action, err := bindingFor(handle).bridge.Message(C.GoString(source), mainFrame != 0, C.GoBytes(unsafe.Pointer(data), length))
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
	if kind != 1 && kind != 2 && kind != 3 && kind != 4 {
		return 0
	}
	action := "detach"
	if kind == 2 {
		action = "back"
	}
	if kind == 3 {
		action = "settings"
	}
	if kind == 4 {
		action = "reset"
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
func sinanUISessionCall(handle C.ulonglong, source *C.char, mainFrame C.int, data *C.char, length C.int) (result *C.char) {
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
	value, err := binding.bridge.Call(C.GoString(source), mainFrame != 0, C.GoBytes(unsafe.Pointer(data), length))
	if err != nil {
		value, _ = json.Marshal(map[string]string{"error": err.Error()})
	} else {
		value, _ = json.Marshal(map[string]json.RawMessage{"result": value})
	}
	return C.CString(string(value))
}

//export sinanUITheme
func sinanUITheme(handle C.ulonglong, revision C.ulonglong) (result *C.char) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	binding := bindingFor(handle)
	if !binding.session.Active() {
		return nil
	}
	if source, ok := binding.session.(interface {
		ThemeState() *protocol.ThemeState
		ThemeRevision() uint64
	}); ok {
		if source.ThemeRevision() <= uint64(revision) {
			return nil
		}
		if state := source.ThemeState(); state != nil {
			data, err := json.Marshal(state)
			if err == nil {
				return C.CString(string(data))
			}
		}
	}
	return nil
}

//export sinanUIEmbeddedAction
func sinanUIEmbeddedAction(handle C.ulonglong, kind C.int) {
	defer func() { _ = recover() }()
	binding := bindingFor(handle)
	if binding.session.Active() && binding.embeddedAction != nil {
		go binding.embeddedAction(int(kind))
	}
}

//export sinanUISessionNavigate
func sinanUISessionNavigate(handle C.ulonglong, target *C.char, mainFrame C.int) (result C.int) {
	defer func() { _ = recover() }()
	if bindingFor(handle).bridge.Navigate(C.GoString(target), mainFrame != 0) {
		return 1
	}
	return 0
}
