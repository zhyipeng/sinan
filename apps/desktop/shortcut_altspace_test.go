package main

import "testing"

func TestAltSpaceMessageClassification(t *testing.T) {
	cases := []struct {
		name           string
		msg            uint32
		wParam, lParam uintptr
		want           bool
	}{
		{"syscommand space menu", altSpaceWMSysCommand, altSpaceSCKeyMenu, altSpaceVKSpace, true},
		{"syscommand space menu with reserved bits", altSpaceWMSysCommand, altSpaceSCKeyMenu | 0x0F, altSpaceVKSpace, true},
		{"syscommand plain alt", altSpaceWMSysCommand, altSpaceSCKeyMenu, 0, false},
		{"syscommand other sc", altSpaceWMSysCommand, 0xF020, altSpaceVKSpace, false},
		{"syskeydown space", 0x0104, altSpaceVKSpace, 0, false},
		{"keydown space not sys", 0x0100, altSpaceVKSpace, 0, false},
	}
	for _, tc := range cases {
		if got := isAltSpaceShortcutMessage(tc.msg, tc.wParam, tc.lParam); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeShortcutAcceptsAltSpace(t *testing.T) {
	got, err := normalizeShortcut(altSpaceChord)
	if err != nil || got != altSpaceChord {
		t.Fatalf("Alt+Space not accepted: %q %v", got, err)
	}
	if lower, err := normalizeShortcut("alt+space"); err != nil || lower != altSpaceChord {
		t.Fatalf("lowercase Alt+Space not canonical: %q %v", lower, err)
	}
	if !shortcutIsAltSpace(ShortcutState{Shortcut: altSpaceChord}) || !shortcutIsAltSpace(ShortcutState{Active: altSpaceChord}) {
		t.Fatal("configured Alt+Space not detected")
	}
	if shortcutIsAltSpace(ShortcutState{Shortcut: "Ctrl+Space"}) {
		t.Fatal("Ctrl+Space misreported as Alt+Space")
	}
}
