package uihost

import (
	"strconv"
	"strings"
)

type webviewRect struct{ Left, Top, Right, Bottom int32 }

func webviewBodyBounds(width, height int32, dpi uint32, detached bool) webviewRect {
	if dpi < 48 || dpi > 960 {
		dpi = 96
	}
	scale := func(dip int32) int32 { return int32((int64(dip)*int64(dpi) + 95) / 96) }
	width = max(0, width)
	height = max(0, height)
	top := min(height, scale(58))
	bottom := height
	if !detached {
		bottom = min(bottom, scale(600))
	}
	return webviewRect{Top: top, Right: width, Bottom: bottom}
}

// Modifiers are normalized independently of Win32: ctrl, shift, alt, meta.
const (
	webviewCtrl uint32 = 1 << iota
	webviewShift
	webviewAlt
	webviewMeta
	webviewAltGraph
)

func webviewPhysicalKey(vk, scan uint32) string {
	const digits = "1234567890"
	if scan >= 2 && scan <= 11 {
		return string(digits[scan-2])
	}
	for _, row := range []struct {
		first uint32
		keys  string
	}{{0x10, "QWERTYUIOP"}, {0x1e, "ASDFGHJKL"}, {0x2c, "ZXCVBNM"}} {
		if scan >= row.first && scan < row.first+uint32(len(row.keys)) {
			return string(row.keys[scan-row.first])
		}
	}
	if scan == 0x33 {
		return "Comma"
	}
	switch vk {
	case 0x1b:
		return "Escape"
	case 0x20:
		return "Space"
	case 0x0d:
		return "Enter"
	case 0x09:
		return "Tab"
	case 0x08:
		return "Backspace"
	case 0x2e:
		return "Delete"
	case 0x24:
		return "Home"
	case 0x23:
		return "End"
	case 0x21:
		return "Page Up"
	case 0x22:
		return "Page Down"
	case 0x26:
		return "Up"
	case 0x28:
		return "Down"
	case 0x25:
		return "Left"
	case 0x27:
		return "Right"
	}
	if vk >= 0x70 && vk <= 0x87 {
		return "F" + strconv.Itoa(int(vk-0x6f))
	}
	return ""
}
func webviewShortcutAction(shortcut string, vk, scan, mods uint32, repeat bool) int {
	if repeat {
		return 0
	}
	key := webviewPhysicalKey(vk, scan)
	if key == "Escape" && mods == 0 {
		return 3
	}
	if key == "Comma" && mods == webviewCtrl {
		return 2
	}
	var expected uint32
	parts := strings.Split(shortcut, "+")
	if len(parts) < 2 {
		return 0
	}
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Ctrl", "Control":
			expected |= webviewCtrl
		case "Shift":
			expected |= webviewShift
		case "Alt", "Option":
			expected |= webviewAlt
		case "Super", "Meta", "Win", "Command", "Cmd":
			expected |= webviewMeta
		default:
			return 0
		}
	}
	if key != "" && key == parts[len(parts)-1] && expected == mods {
		return 1
	}
	return 0
}
