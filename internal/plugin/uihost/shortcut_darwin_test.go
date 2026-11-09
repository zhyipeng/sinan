//go:build darwin && !ios && cgo

package uihost

import "testing"

func TestNativeDetachShortcutMatchesConfiguredPhysicalKeys(t *testing.T) {
	const shift, ctrl, alt, cmd = 1 << 17, 1 << 18, 1 << 19, 1 << 20
	for _, test := range []struct {
		value string
		code  uint16
		flags uint64
		want  bool
	}{
		{"Shift+Cmd+D", 2, shift | cmd, true},
		{"Ctrl+Alt+Shift+U", 32, ctrl | alt | shift, true},
		{"Ctrl+Alt+Shift+U", 2, shift | cmd, false},
		{"Ctrl+Alt+Shift+U", 32, ctrl | alt, false},
		{"Ctrl+Alt+F19", 80, ctrl | alt, true},
		{"Cmd+Left", 123, cmd, true},
		{"Ctrl+1", 18, ctrl, true},
		{"Cmd+Unknown", 0, cmd, false},
		{"Cmd+K", 40, ctrl, false},
	} {
		if got := matchesNativeDetachKey(test.value, test.code, test.flags); got != test.want {
			t.Fatalf("native binding mismatch for %s", test.value)
		}
	}
}
