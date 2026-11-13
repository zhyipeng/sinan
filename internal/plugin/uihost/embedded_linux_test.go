//go:build linux && cgo && gtk3

package uihost

import "testing"

func TestGTKContentKeepsTrustedHeaderAndDetachedHeight(t *testing.T) {
	for _, tc := range []struct {
		height   int
		detached bool
		want     int
	}{
		{50, false, 0}, {58, false, 0}, {488, false, 430}, {600, false, 542}, {800, false, 542}, {800, true, 742},
	} {
		if got := gtkBodyHeight(tc.height, tc.detached); got != tc.want {
			t.Fatalf("GTK content height %d, want %d", got, tc.want)
		}
	}
}

func TestGTKConfiguredShortcutKeysAndExactModifiers(t *testing.T) {
	const shift, caps, ctrl, alt, super = 1, 2, 4, 8, 1 << 26
	for _, tc := range []struct {
		shortcut, key string
		modifiers     uint32
		want          bool
	}{
		{"Ctrl+Shift+D", "D", ctrl | shift, true},
		{"Ctrl+Shift+D", "d", ctrl | shift | caps, true},
		{"Ctrl+Shift+D", "d", ctrl, false},
		{"Ctrl+Shift+D", "d", ctrl | shift | alt, false},
		{"Super+Left", "Left", super, true},
		{"Ctrl+Comma", "comma", ctrl, true},
		{"Ctrl+Enter", "KP_Enter", ctrl, true},
		{"Ctrl+Shift+Tab", "ISO_Left_Tab", ctrl | shift, true},
		{"Ctrl+Page Down", "Page_Down", ctrl, true},
		{"Ctrl+Backspace", "BackSpace", ctrl, true},
		{"Ctrl+Alt+F19", "F19", ctrl | alt, true},
		{"Ctrl+Unknown", "d", ctrl, false},
	} {
		if got := matchesGTKDetachKey(tc.shortcut, tc.key, tc.modifiers); got != tc.want {
			t.Fatalf("GTK shortcut %s mismatch", tc.shortcut)
		}
	}
}

func TestGTKShortcutsUseRecordedPhysicalPositions(t *testing.T) {
	const ctrl, shift, altGraph = 4, 1, 1 << 7
	for _, tc := range []struct {
		shortcut, key       string
		modifiers, hardware uint32
		want                bool
	}{
		{"Ctrl+D", "e", ctrl, 40, true},
		{"Ctrl+D", "d", ctrl, 26, false},
		{"Ctrl+Shift+1", "exclam", ctrl | shift, 10, true},
		{"Ctrl+Comma", "w", ctrl, 59, true},
		{"Ctrl+Shift+D", "D", ctrl | shift | altGraph, 40, false},
	} {
		if matchesGTKDetachKey(tc.shortcut, tc.key, tc.modifiers, tc.hardware) != tc.want {
			t.Fatalf("GTK physical key mismatch for %s", tc.shortcut)
		}
	}
}
