//go:build (!darwin && !windows && !linux) || ios || (darwin && !cgo) || (linux && (!cgo || !gtk3))

package platform

import "unsafe"

// CursorScreenID is not implemented on these platforms yet. The launcher falls
// back to its current display without guessing physical versus logical pixels.
func CursorScreenID() (string, bool) { return "", false }

func ScreenWorkArea(id string) (WorkArea, bool) { return WorkArea{}, false }

func DetachPanel(window unsafe.Pointer) {}
