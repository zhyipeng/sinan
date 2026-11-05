//go:build !darwin || ios || !cgo

package main

import "errors"

func runNative([]byte, string, viewSession, func() error) (report, error) {
	return report{}, errors.New("native UI probe requires macOS with cgo")
}
