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
	"testing"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
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
	icon := readIcon(iconPath)
	img := decodePlatformIcon(t, icon)
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatal("transparent corner became opaque")
	}
	r, g, b, a := img.At(applicationIconSize/2, applicationIconSize/2).RGBA()
	if r < 0xf000 || g > 0x1000 || b > 0x1000 || a < 0xf000 {
		t.Fatalf("custom icon color was not preserved: %d %d %d %d", r, g, b, a)
	}
	shortcut := filepath.Join(t.TempDir(), "自定义快捷方式.lnk")
	writePlatformShortcut(t, shortcut, executable, iconPath)
	if got := readIcon(shortcut); got != icon {
		t.Fatal("shortcut did not resolve its custom icon")
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
	object, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Release()
	shell, err := object.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Release()
	value, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Clear()
	link := value.ToIDispatch()
	for key, property := range map[string]string{"TargetPath": target, "IconLocation": icon + ",0"} {
		result, err := oleutil.PutProperty(link, key, property)
		if err != nil {
			t.Fatal(err)
		}
		result.Clear()
	}
	result, err := oleutil.CallMethod(link, "Save")
	if err != nil {
		t.Fatal(err)
	}
	result.Clear()
}
