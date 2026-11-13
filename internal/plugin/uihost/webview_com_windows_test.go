//go:build windows && (amd64 || arm64) && !native_webview2loader

package uihost

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These tests call only in-process COM stubs, never the WebView2 loader, a
// browser process, a native window, or a message loop.
func TestWebviewCOMCallbackNativeLeaseSurvivesGC(t *testing.T) {
	var calls atomic.Int32
	value := uint32(41)
	callback := newWebviewCallback("49511172-cc67-4bca-9923-137112f4c4cc", func(code uintptr, result unsafe.Pointer) uintptr {
		if code == 17 && (*uint32)(result) == &value {
			calls.Add(1)
		}
		return 0
	})
	pointer := callback.pointer()
	refs, _, _ := syscall.SyscallN(webviewCallbackTable[1], pointer)
	if refs != 2 {
		t.Fatal("native AddRef did not retain callback")
	}
	callback.release()
	callback = nil
	runtime.GC()
	result, _, _ := syscall.SyscallN(webviewCallbackTable[3], pointer, 17, uintptr(unsafe.Pointer(&value)))
	if result != 0 || calls.Load() != 1 {
		t.Fatal("native callback did not survive Go owner release")
	}
	refs, _, _ = syscall.SyscallN(webviewCallbackTable[2], pointer)
	if refs != 0 || callbackFor(pointer) != nil {
		t.Fatal("last COM Release leaked callback")
	}
}

func TestWebviewCOMQueryRejectsUnimplementedInterfaces(t *testing.T) {
	callback := newWebviewCallback("49511172-cc67-4bca-9923-137112f4c4cc", func(uintptr, unsafe.Pointer) uintptr { return 0 })
	defer callback.release()
	object := (*webviewCOM)(unsafe.Pointer(callback))
	for _, id := range []string{"00000000-0000-0000-c000-000000000046", "49511172-cc67-4bca-9923-137112f4c4cc"} {
		queried, err := object.query(id)
		if err != nil || queried != object {
			t.Fatal("implemented callback IID rejected", err)
		}
		queried.release()
	}
	// Claiming IAgileObject would falsely permit the UI-thread closure to run on
	// an arbitrary COM apartment. It must be rejected, along with unrelated IIDs.
	for _, id := range []string{"94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90", "76eceacb-0462-4d94-ac83-423a6793775e"} {
		queried, err := object.query(id)
		if err == nil || queried != nil {
			t.Fatal("unimplemented callback IID accepted")
		}
	}
	if callback.refs.Load() != 1 {
		t.Fatal("QueryInterface references not balanced")
	}
	var out uintptr = 1
	iid, _ := windows.GUIDFromString("{94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90}")
	result, _, _ := syscall.SyscallN(webviewCallbackTable[0], callback.pointer(), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
	if uint32(result) != 0x80004002 || out != 0 {
		t.Fatal("failed QI did not clear output pointer")
	}
}

// A failed request getter must not leave WebResourceRequested.Response unset.
// These native stubs contain no engine and do not create any windows.
type webviewResourceStub struct {
	table     *[128]uintptr
	response  *webviewCOM
	installed bool
	status    uintptr
}

var webviewResourceStubTable = func() *[128]uintptr {
	table := new([128]uintptr)
	table[2] = syscall.NewCallback(func(*webviewResourceStub) uintptr { return 1 })
	table[3] = syscall.NewCallback(func(*webviewResourceStub, **webviewCOM) uintptr { return 0x80004005 })
	table[4] = syscall.NewCallback(func(this *webviewResourceStub, stream, status uintptr, reason, headers *uint16, out **webviewCOM) uintptr {
		this.status = status
		*out = this.response
		return 0
	})
	table[5] = syscall.NewCallback(func(this *webviewResourceStub, response *webviewCOM) uintptr {
		this.installed = response == this.response
		return 0
	})
	return table
}()

func TestWebviewResourceInstallsDenialBeforeRequestFailure(t *testing.T) {
	b, _ := bridgeFixture(t)
	response := &webviewCOM{table: webviewResourceStubTable}
	environment := &webviewResourceStub{table: webviewResourceStubTable, response: response}
	args := &webviewResourceStub{table: webviewResourceStubTable, response: response}
	e := &Embedded{transport: newWebviewTransport(b), env: (*webviewCOM)(unsafe.Pointer(environment))}
	err := e.resource((*webviewCOM)(unsafe.Pointer(args)))
	if err == nil {
		t.Fatal("failed getter not propagated")
	}
	if !args.installed || environment.status != 404 {
		t.Fatal("failed request was left to the network")
	}
	runtime.KeepAlive(environment)
	runtime.KeepAlive(args)
	runtime.KeepAlive(response)
}
