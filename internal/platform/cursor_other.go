//go:build !darwin || ios || !cgo

package platform

// CursorScreenID is not implemented on these platforms yet. The launcher falls
// back to its current display without guessing physical versus logical pixels.
func CursorScreenID() (string, bool) { return "", false }
