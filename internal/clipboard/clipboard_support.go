package clipboard

import (
	"errors"
	"strings"
)

// Only fixed, host-owned messages may cross the backend error boundary.
type clipboardSupportError string

func (e clipboardSupportError) Error() string { return string(e) }
func clipboardStatus(err error) string {
	var support clipboardSupportError
	if errors.As(err, &support) {
		return string(support)
	}
	return "无法访问系统剪贴板，请检查系统支持、占用或权限"
}
func clipboardMIME(types string) (kind, mime string) {
	offered := map[string]bool{}
	for _, value := range strings.Fields(types) {
		offered[value] = true
	}
	if offered["image/png"] {
		return "image", "image/png"
	}
	for _, value := range []string{"text/plain;charset=utf-8", "text/plain;charset=UTF-8", "UTF8_STRING", "text/plain"} {
		if offered[value] {
			return "text", value
		}
	}
	return "", ""
}
