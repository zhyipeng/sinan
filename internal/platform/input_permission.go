package platform

import "errors"

// ErrAccessibilityPermission distinguishes missing OS input consent from stale focus.
var ErrAccessibilityPermission = errors.New("自动粘贴需要辅助功能权限，请在系统设置中为司南开启")
