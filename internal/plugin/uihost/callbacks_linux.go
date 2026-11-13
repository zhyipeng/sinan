//go:build linux && cgo && gtk3

package uihost

/*
#include <stdlib.h>
#include "native_linux.h"
*/
import "C"

import (
	"encoding/json"
	"runtime/cgo"
	"unsafe"

	"sinan/internal/plugin/uiview"
)

func gtkBindingFor(handle C.ulonglong) *gtkBinding { return cgo.Handle(handle).Value().(*gtkBinding) }

//export sinanGTKResource
func sinanGTKResource(handle C.ulonglong, method, url *C.char, mime **C.char, length *C.int) (result unsafe.Pointer) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	resource, ok := gtkBindingFor(handle).bridge.Resource(C.GoString(method), C.GoString(url))
	if !ok {
		return nil
	}
	*mime = C.CString(resource.MIME)
	*length = C.int(len(resource.Data))
	if len(resource.Data) == 0 {
		return C.malloc(1)
	}
	return C.CBytes(resource.Data)
}

//export sinanGTKMessage
func sinanGTKMessage(handle C.ulonglong, source, data *C.char, length C.int) (result C.int) {
	defer func() { _ = recover() }()
	if length < 0 || length > uiview.MaxMessageBytes {
		return 0
	}
	action, err := gtkBindingFor(handle).bridge.Message(C.GoString(source), true, C.GoBytes(unsafe.Pointer(data), length))
	if err != nil {
		return 0
	}
	return C.int(action)
}

//export sinanGTKCall
func sinanGTKCall(handle C.ulonglong, source, data *C.char, length C.int, view *C.WebKitWebView, reply *C.WebKitScriptMessageReply, context *C.JSCContext) (result C.int) {
	defer func() { _ = recover() }()
	if length < 0 || length > uiview.MaxCallBytes {
		return 0
	}
	b := gtkBindingFor(handle)
	input, origin := C.GoBytes(unsafe.Pointer(data), length), C.GoString(source)
	go func() {
		value, err := b.bridge.Call(origin, true, input)
		failed := C.int(0)
		if err != nil {
			value, _ = json.Marshal("UI 调用失败或会话已关闭")
			failed = 1
		}
		encoded := C.CString(string(value))
		defer C.free(unsafe.Pointer(encoded))
		b.invoke(func() { C.sinanGTKReply(view, reply, context, encoded, failed) })
	}()
	return 1
}

//export sinanGTKNavigate
func sinanGTKNavigate(handle C.ulonglong, target *C.char) (result C.int) {
	defer func() { _ = recover() }()
	// This event is only allowed for the host's initial load; all subsequent
	// navigation and all new-window decisions are rejected in the native layer.
	if gtkBindingFor(handle).bridge.Navigate(C.GoString(target), true) {
		return 1
	}
	return 0
}

//export sinanGTKRevoke
func sinanGTKRevoke(handle C.ulonglong) {
	defer func() { _ = recover() }()
	gtkBindingFor(handle).bridge.session.Revoke()
}

//export sinanGTKAction
func sinanGTKAction(handle C.ulonglong, kind C.int) {
	defer func() { _ = recover() }()
	b := gtkBindingFor(handle)
	if b.action != nil {
		go func() {
			if b.bridge.session.Active() {
				b.action(int(kind))
			}
		}()
	}
}
