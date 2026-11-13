//go:build (!darwin && !windows && !linux) || ios || (darwin && !cgo) || (linux && (!cgo || !gtk3))

package platform

import "unsafe"

func SystemDark() bool                                  { return false }
func SetWindowTheme(window unsafe.Pointer, mode string) {}
