//go:build (!darwin && !windows && !linux) || (darwin && (!cgo || ios))

package platform

import "errors"

func CaptureInputTarget() uintptr         { return 0 }
func CheckPasteTarget(uintptr) error      { return errors.New("此平台暂不支持自动粘贴") }
func ActivateInputTarget(t uintptr) error { return CheckPasteTarget(t) }
func SendPaste(t uintptr) error           { return CheckPasteTarget(t) }
