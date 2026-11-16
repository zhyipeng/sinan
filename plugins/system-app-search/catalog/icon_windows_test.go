package catalog

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// This exercises only offscreen GDI bitmaps and Shell metadata; it never opens
// an application, a window, or a WebView.
func TestPlatformWindowsApplicationIcons(t *testing.T) {
	if readIcon("invalid\x00.exe") != "" || encodeWindowsIcon(0) != "" {
		t.Fatal("invalid icon input did not fall back")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	decodePlatformIcon(t, readIcon(executable))
	iconPath := filepath.Join(t.TempDir(), "自定义图标.ico")
	writePlatformIcon(t, iconPath)
	checkPlatformCustomIcon(t, readIcon(iconPath))
	shortcut := filepath.Join(t.TempDir(), "自定义快捷方式.lnk")
	writePlatformShortcut(t, shortcut, executable, iconPath)
	// The Shell may render a shortcut overlay or use another raster size.
	// Check the fixture's visible artwork instead of requiring identical PNGs.
	checkPlatformCustomIcon(t, readIcon(shortcut))
}

func checkPlatformCustomIcon(t *testing.T, value string) {
	t.Helper()
	img := decodePlatformIcon(t, value)
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("transparent corner became opaque")
	}
	// The red interior distinguishes the custom fixture from the executable's
	// default icon while leaving the lower-left shortcut overlay unconstrained.
	for y := 20; y < 32; y++ {
		for x := 24; x < 40; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r < 0xf000 || g > 0x1000 || b > 0x1000 || a < 0xf000 {
				t.Fatalf("custom icon color was not preserved at %d,%d: %d %d %d %d", x, y, r, g, b, a)
			}
		}
	}
}

func decodePlatformIcon(t *testing.T, value string) image.Image {
	t.Helper()
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(value, prefix) {
		t.Fatal("missing PNG data URL")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, applicationIconSize, applicationIconSize) {
		t.Fatalf("unexpected icon bounds: %v", img.Bounds())
	}
	return img
}

func writePlatformIcon(t *testing.T, path string) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 16; y < 48; y++ {
		for x := 16; x < 48; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	// ICONDIR followed by one ICONDIRENTRY with a PNG payload.
	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], 1)
	header[6], header[7] = 64, 64
	binary.LittleEndian.PutUint16(header[10:], 1)
	binary.LittleEndian.PutUint16(header[12:], 32)
	binary.LittleEndian.PutUint32(header[14:], uint32(data.Len()))
	binary.LittleEndian.PutUint32(header[18:], 22)
	if err := os.WriteFile(path, append(header, data.Bytes()...), 0600); err != nil {
		t.Fatal(err)
	}
}

func writePlatformShortcut(t *testing.T, path, target, icon string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			t.Fatal(err)
		}
	}
	defer ole.CoUninitialize()
	// Request the Unicode Shell interface explicitly. The WScript.Shell fixture
	// failed to save Chinese shortcut names on both Windows CI architectures.
	// Keep all fixture paths in UTF-16 instead of depending on script automation.
	link, err := ole.CreateInstance(
		ole.NewGUID("{00021401-0000-0000-C000-000000000046}"), // CLSID_ShellLink
		ole.NewGUID("{000214F9-0000-0000-C000-000000000046}"), // IID_IShellLinkW
	)
	if err != nil {
		t.Fatal(err)
	}
	defer link.Release()
	linkTable := (*platformShellLinkVtbl)(unsafe.Pointer(link.RawVTable))
	targetPath, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	hr, _, _ := syscall.SyscallN(linkTable.SetPath, uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(targetPath)))
	checkPlatformShortcutResult(t, "IShellLinkW.SetPath", hr)
	runtime.KeepAlive(targetPath)
	iconPath, err := windows.UTF16PtrFromString(icon)
	if err != nil {
		t.Fatal(err)
	}
	hr, _, _ = syscall.SyscallN(linkTable.SetIconLocation, uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(iconPath)), 0)
	checkPlatformShortcutResult(t, "IShellLinkW.SetIconLocation", hr)
	runtime.KeepAlive(iconPath)

	var file *ole.IUnknown
	if err := link.PutQueryInterface(ole.NewGUID("{0000010B-0000-0000-C000-000000000046}"), &file); err != nil { // IID_IPersistFile
		t.Fatal(err)
	}
	defer file.Release()
	fileTable := (*platformPersistFileVtbl)(unsafe.Pointer(file.RawVTable))
	shortcutPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	hr, _, _ = syscall.SyscallN(fileTable.Save, uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(shortcutPath)), 1)
	checkPlatformShortcutResult(t, "IPersistFile.Save", hr)
	runtime.KeepAlive(shortcutPath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shortcut was not saved at its Unicode path: %v", err)
	}
}

// Test-only COM layouts, in Windows SDK declaration order. Both interfaces
// inherit IUnknown; IPersistFile also includes IPersist.GetClassID.
// https://learn.microsoft.com/windows/win32/api/shobjidl_core/nn-shobjidl_core-ishelllinkw
// https://learn.microsoft.com/windows/win32/api/objidl/nn-objidl-ipersistfile
type platformShellLinkVtbl struct {
	ole.IUnknownVtbl
	GetPath, GetIDList, SetIDList            uintptr
	GetDescription, SetDescription           uintptr
	GetWorkingDirectory, SetWorkingDirectory uintptr
	GetArguments, SetArguments               uintptr
	GetHotkey, SetHotkey                     uintptr
	GetShowCmd, SetShowCmd                   uintptr
	GetIconLocation, SetIconLocation         uintptr
	SetRelativePath, Resolve, SetPath        uintptr
}

type platformPersistFileVtbl struct {
	ole.IUnknownVtbl
	GetClassID, IsDirty, Load, Save uintptr
}

func checkPlatformShortcutResult(t *testing.T, operation string, result uintptr) {
	t.Helper()
	if result != 0 {
		t.Fatalf("%s failed: %v", operation, ole.NewError(result))
	}
}
