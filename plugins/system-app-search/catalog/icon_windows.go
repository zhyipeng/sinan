package catalog

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"runtime"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	iconShell32      = windows.NewLazySystemDLL("shell32.dll")
	iconUser32       = windows.NewLazySystemDLL("user32.dll")
	iconGDI32        = windows.NewLazySystemDLL("gdi32.dll")
	getFileIcon      = iconShell32.NewProc("SHGetFileInfoW")
	destroyIcon      = iconUser32.NewProc("DestroyIcon")
	drawIcon         = iconUser32.NewProc("DrawIconEx")
	createIconDC     = iconGDI32.NewProc("CreateCompatibleDC")
	deleteIconDC     = iconGDI32.NewProc("DeleteDC")
	createIconDIB    = iconGDI32.NewProc("CreateDIBSection")
	selectIconObject = iconGDI32.NewProc("SelectObject")
	deleteIconObject = iconGDI32.NewProc("DeleteObject")
	flushIconGDI     = iconGDI32.NewProc("GdiFlush")
)

type shellIconInfo struct {
	Icon        uintptr
	Index       int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}

type iconBitmapInfo struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
	Colors                       [1]uint32
}

func readIcon(path string) string {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	// Shell icon handlers need a COM apartment on a stable background thread.
	// Query the actual entry (without USEFILEATTRIBUTES), so .lnk custom icons
	// and target resources are resolved by the Shell rather than by file type.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			return ""
		}
	}
	defer ole.CoUninitialize()
	var info shellIconInfo
	ok, _, _ := getFileIcon.Call(uintptr(unsafe.Pointer(file)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0x100) // SHGFI_ICON | SHGFI_LARGEICON
	if info.Icon != 0 {
		defer destroyIcon.Call(info.Icon)
	}
	if ok == 0 || info.Icon == 0 {
		return ""
	}
	return encodeWindowsIcon(info.Icon)
}

func encodeWindowsIcon(icon uintptr) string {
	if icon == 0 {
		return ""
	}
	dc, _, _ := createIconDC.Call(0)
	if dc == 0 {
		return ""
	}
	defer deleteIconDC.Call(dc)
	// A negative height requests top-down BGRA pixels, avoiding a vertical flip.
	info := iconBitmapInfo{Size: 40, Width: applicationIconSize, Height: -applicationIconSize, Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := createIconDIB.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return ""
	}
	defer deleteIconObject.Call(bitmap)
	if bits == nil {
		return ""
	}
	old, _, _ := selectIconObject.Call(dc, bitmap)
	if old == 0 || old == ^uintptr(0) {
		return ""
	}
	defer selectIconObject.Call(dc, old)
	pixels := unsafe.Slice((*byte)(bits), applicationIconSize*applicationIconSize*4)
	// DrawIconEx supports both alpha icons and older AND/XOR masks. Rendering
	// over black and white recovers transparency without trusting the DIB alpha
	// byte, which GDI does not consistently preserve for legacy icons.
	render := func(background byte) []byte {
		for i := range pixels {
			pixels[i] = background
		}
		ok, _, _ := drawIcon.Call(dc, 0, 0, icon, applicationIconSize, applicationIconSize, 0, 0, 3) // DI_NORMAL
		flushed, _, _ := flushIconGDI.Call()
		if ok == 0 || flushed == 0 {
			return nil
		}
		return bytes.Clone(pixels)
	}
	black := render(0)
	if black == nil {
		return ""
	}
	white := render(255)
	if white == nil {
		return ""
	}
	img := windowsIconPixels(black, white)
	var output bytes.Buffer
	if png.Encode(&output, img) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes())
}
