package main

// Windows treats Alt+Space as the window system menu (SC_KEYMENU), so the
// combination never reaches the recorder and, on a focused Sinan window, pops
// the system menu even when a global hot key is registered. These helpers let
// the Windows window proc suppress that menu and capture the chord natively.
//
// The numeric message values keep this file platform independent so it can be
// unit tested everywhere; only the Windows build installs the interceptor.
const (
	altSpaceWMSysCommand = 0x0112 // WM_SYSCOMMAND
	altSpaceSCKeyMenu    = 0xF100 // SC_KEYMENU
	altSpaceVKSpace      = 0x20   // VK_SPACE
	altSpaceChord        = "Alt+Space"
)

// isAltSpaceShortcutMessage reports whether msg is the system-menu activation
// Windows generates for Alt+Space. WM_SYSCOMMAND carries the character in the
// low word of lParam; plain Alt/F10 uses lParam 0, so requiring the space key
// distinguishes it from an ordinary menu activation.
func isAltSpaceShortcutMessage(msg uint32, wParam, lParam uintptr) bool {
	if msg != altSpaceWMSysCommand {
		return false
	}
	return uint32(wParam)&0xFFF0 == altSpaceSCKeyMenu && lParam&0xFFFF == uintptr(altSpaceVKSpace)
}

func shortcutIsAltSpace(state ShortcutState) bool {
	return state.Shortcut == altSpaceChord || state.Active == altSpaceChord
}

// updateAltSpaceHotkey refreshes the lock-free flag the Windows window proc
// reads. It runs from shortcut change callbacks, never from the UI thread, so
// taking the shortcut locks here cannot deadlock against the message loop.
func (h *HostService) updateAltSpaceHotkey() {
	configured := false
	if h.shortcuts != nil {
		configured = shortcutIsAltSpace(h.shortcuts.State())
	}
	if !configured && h.detachShortcuts != nil {
		configured = shortcutIsAltSpace(h.detachShortcuts.State())
	}
	if !configured {
		h.shortcutConfigMu.Lock()
		for _, entry := range h.pluginShortcuts {
			if shortcutIsAltSpace(entry.controller.State()) {
				configured = true
				break
			}
		}
		h.shortcutConfigMu.Unlock()
	}
	h.altSpaceHotkey.Store(configured)
}
