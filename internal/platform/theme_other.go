//go:build !darwin || ios || !cgo

package platform

import "unsafe"

func SystemDark() bool                                  { return false }
func SetWindowTheme(window unsafe.Pointer, mode string) {}
