//go:build !darwin || ios || !cgo

package uihost

import "errors"

const Supported = false

func runNative([]byte, string, Session, func() error) (Report, error) {
	return Report{}, errors.New("native UI requires macOS with cgo")
}
