package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

func readMetadata(path string) (metadata, error) {
	if !applicationEntry("windows", path, false) {
		return metadata{}, errors.New("unsupported application entry")
	}
	return metadata{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}, nil
}

func readIcon(string) string { return "" }

func launchApplication(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// ShellExecute opens the indexed shortcut/executable without cmd.exe parsing.
	// Shell COM handlers require an initialized apartment on a stable OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			return err
		}
	}
	defer ole.CoUninitialize()
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, nil, file, nil, nil, windows.SW_SHOWNORMAL)
}
