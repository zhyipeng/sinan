//go:build windows && (amd64 || arm64) && !native_webview2loader

package uihost

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// COM slots and IIDs below follow Microsoft.Web.WebView2 1.0.2210.55,
// build/native/include/WebView2.h. Use only integer/pointer ABI arguments here;
// BOOL is a value, HWND is a value, and COM out parameters are pointer-to-pointer.
// The upstream loader is reused, but its generated interface wrappers are not.
type webviewCOM struct{ table *[128]uintptr }

//go:uintptrescapes
func (o *webviewCOM) call(slot int, args ...uintptr) error {
	if o == nil {
		return errors.New("WebView2 接口不可用")
	}
	values := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	result, _, _ := syscall.SyscallN(o.table[slot], values...)
	runtime.KeepAlive(o)
	if int32(result) < 0 {
		return errors.New("WebView2 原生操作失败")
	}
	return nil
}
func (o *webviewCOM) release() {
	if o != nil {
		_ = o.call(2)
	}
}
func (o *webviewCOM) query(id string) (*webviewCOM, error) {
	iid, err := windows.GUIDFromString("{" + id + "}")
	if err != nil {
		return nil, err
	}
	var out *webviewCOM
	err = o.call(0, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}
func (o *webviewCOM) object(slot int) (*webviewCOM, error) {
	var out *webviewCOM
	err := o.call(slot, uintptr(unsafe.Pointer(&out)))
	if err == nil && out == nil {
		err = errors.New("WebView2 接口为空")
	}
	return out, err
}
func (o *webviewCOM) number(slot int) (uint32, error) {
	var out uint32
	err := o.call(slot, uintptr(unsafe.Pointer(&out)))
	return out, err
}
func (o *webviewCOM) text(slot, limit int) (string, error) {
	var out *uint16
	if err := o.call(slot, uintptr(unsafe.Pointer(&out))); err != nil {
		return "", err
	}
	if out == nil {
		return "", errors.New("WebView2 字符串为空")
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(out))
	return webviewUTF16Text(out, limit)
}
func webviewUTF16Text(out *uint16, limit int) (string, error) {
	if out == nil {
		return "", errors.New("WebView2 字符串为空")
	}
	// Native strings are UTF-16 NUL terminated. Bound scans before allocation.
	for i := 0; i <= limit; i++ {
		if *(*uint16)(unsafe.Add(unsafe.Pointer(out), i*2)) == 0 {
			return windows.UTF16ToString(unsafe.Slice(out, i)), nil
		}
	}
	return "", errors.New("WebView2 字符串过大")
}

func webviewString(s string) *uint16 { value, _ := windows.UTF16PtrFromString(s); return value }

// Every callback has one Go owner reference and the references held by COM.
// Registration pins the object and roots the closure until the last Release.
// Dispatch never reconstructs a Go pointer from an untrusted/native integer.
type webviewCallback struct {
	table *[4]uintptr
	refs  atomic.Int32
	pin   runtime.Pinner
	iid   windows.GUID
	run   func(uintptr, unsafe.Pointer) uintptr
}

var webviewCallbacks sync.Map
var webviewCallbackTable = [4]uintptr{
	syscall.NewCallback(webviewCallbackQuery), syscall.NewCallback(webviewCallbackAddRef), syscall.NewCallback(webviewCallbackRelease), syscall.NewCallback(webviewCallbackInvoke),
}

func newWebviewCallback(id string, run func(uintptr, unsafe.Pointer) uintptr) *webviewCallback {
	iid, err := windows.GUIDFromString("{" + id + "}")
	if err != nil {
		panic("invalid WebView2 IID")
	}
	c := &webviewCallback{table: &webviewCallbackTable, iid: iid, run: run}
	c.refs.Store(1)
	c.pin.Pin(c)
	webviewCallbacks.Store(uintptr(unsafe.Pointer(c)), c)
	return c
}
func callbackFor(this uintptr) *webviewCallback {
	value, ok := webviewCallbacks.Load(this)
	if !ok {
		return nil
	}
	return value.(*webviewCallback)
}
func webviewCallbackQuery(this uintptr, iid *windows.GUID, out *uintptr) uintptr {
	c := callbackFor(this)
	if c == nil || out == nil || iid == nil {
		return 0x80004003
	}
	result := out
	*result = 0
	requested := *iid
	unknown := windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	if requested != c.iid && requested != unknown {
		return 0x80004002
	}
	c.refs.Add(1)
	*result = this
	return 0
}
func webviewCallbackAddRef(this uintptr) uintptr {
	if c := callbackFor(this); c != nil {
		return uintptr(c.refs.Add(1))
	}
	return 0
}
func webviewCallbackRelease(this uintptr) uintptr {
	c := callbackFor(this)
	if c == nil {
		return 0
	}
	n := c.refs.Add(-1)
	if n == 0 {
		webviewCallbacks.Delete(this)
		c.pin.Unpin()
	}
	return uintptr(n)
}
func webviewCallbackInvoke(this, a uintptr, b unsafe.Pointer) (result uintptr) {
	result = 0x80004005
	defer func() { _ = recover() }()
	if c := callbackFor(this); c != nil {
		return c.run(a, b)
	}
	return result
}
func (c *webviewCallback) pointer() uintptr { return uintptr(unsafe.Pointer(c)) }
func (c *webviewCallback) release()         { webviewCallbackRelease(c.pointer()) }
