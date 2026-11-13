package uihost

import "testing"

func TestWebviewBodyBoundsKeepHeaderInPhysicalPixels(t *testing.T) {
	for _, tc := range []struct {
		w, h     int32
		dpi      uint32
		detached bool
		want     webviewRect
	}{
		{800, 50, 96, false, webviewRect{Top: 50, Right: 800, Bottom: 50}},
		{800, 800, 96, false, webviewRect{Top: 58, Right: 800, Bottom: 600}},
		{800, 800, 96, true, webviewRect{Top: 58, Right: 800, Bottom: 800}},
		{1200, 1200, 144, false, webviewRect{Top: 87, Right: 1200, Bottom: 900}},
		{1000, 1000, 120, false, webviewRect{Top: 73, Right: 1000, Bottom: 750}},
		{-5, -10, 96, false, webviewRect{}},
		{800, 800, 0, false, webviewRect{Top: 58, Right: 800, Bottom: 600}},
	} {
		if got := webviewBodyBounds(tc.w, tc.h, tc.dpi, tc.detached); got != tc.want {
			t.Fatalf("bounds %+v, want %+v", got, tc.want)
		}
	}
}
func TestWebviewShortcutsUsePhysicalKeysAndExactModifiers(t *testing.T) {
	for _, tc := range []struct {
		shortcut       string
		vk, scan, mods uint32
		repeat         bool
		want           int
	}{
		{"Ctrl+Shift+D", 'E', 0x20, webviewCtrl | webviewShift, false, 1},
		{"Ctrl+Shift+D", 'D', 0x12, webviewCtrl | webviewShift, false, 0},
		{"Ctrl+Shift+1", '!', 2, webviewCtrl | webviewShift, false, 1},
		{"Ctrl+D", 'D', 0x20, webviewCtrl | webviewAlt, false, 0},
		{"Ctrl+Alt+D", 'D', 0x20, webviewCtrl | webviewAlt | webviewAltGraph, false, 0},
		{"Ctrl+D", 'D', 0x20, webviewCtrl, true, 0},
		{"Ctrl+D", 'D', 0x20, webviewCtrl | webviewMeta, false, 0},
		{"Ctrl+Alt+F19", 0x82, 0, webviewCtrl | webviewAlt, false, 1},
		{"Super+Left", 0x25, 0x4b, webviewMeta, false, 1},
		{"Ctrl+Page Down", 0x22, 0x51, webviewCtrl, false, 1},
		{"Ctrl+Shift+D", 0x1b, 1, 0, false, 3},
		{"Ctrl+Shift+D", 0x1b, 1, webviewAlt, false, 0},
		{"Ctrl+Shift+D", 'W', 0x33, webviewCtrl, false, 2},
		{"Ctrl+Shift+D", 'W', 0x33, webviewCtrl | webviewShift, false, 0},
	} {
		if got := webviewShortcutAction(tc.shortcut, tc.vk, tc.scan, tc.mods, tc.repeat); got != tc.want {
			t.Fatalf("%s vk=%d scan=%d: %d, want %d", tc.shortcut, tc.vk, tc.scan, got, tc.want)
		}
	}
}
