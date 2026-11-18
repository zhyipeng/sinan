//go:build windows && (amd64 || arm64) && !native_webview2loader

package uihost

import (
	"time"
	"unsafe"
)

// Accessed only on the Wails UI thread. Each controller is leased to at most
// one plugin session and is destroyed with that session.
var warmWebviews webviewWarmSlot

type webviewWarmSlot struct {
	hwnd    uintptr
	view    *Embedded
	stopped bool
}

func (s *webviewWarmSlot) take(hwnd uintptr) *Embedded {
	view := s.view
	if s.stopped || hwnd != s.hwnd || view == nil || view.session != nil || view.status < 0 || view.closed.Load() {
		return nil
	}
	s.view = nil
	return view
}

func (s *webviewWarmSlot) clear() {
	if s.view != nil {
		view := s.view
		s.view = nil
		view.closeOnUIThread()
	}
}

func (s *webviewWarmSlot) fill(invoke func(func())) {
	if s.stopped || s.hwnd == 0 || (s.view != nil && !s.view.closed.Load() && s.view.status >= 0) {
		return
	}
	s.clear()
	view := startWebview(s.hwnd, invoke)
	s.view = view
	time.AfterFunc(EmbeddedLoadTimeout, func() {
		if view.closed.Load() {
			return
		}
		invoke(func() {
			if s.view == view && view.creating {
				s.clear()
			}
		})
	})
}

// WarmEmbedded prepares one unused controller for the current launcher HWND.
// The controller stays hidden and receives no plugin content until claimed.
func WarmEmbedded(window func() unsafe.Pointer, invoke func(func())) {
	invoke(func() {
		if warmWebviews.stopped {
			return
		}
		hwnd := uintptr(window())
		if hwnd == 0 {
			return
		}
		if warmWebviews.hwnd != hwnd {
			warmWebviews.clear()
			warmWebviews.hwnd = hwnd
		}
		warmWebviews.fill(invoke)
	})
}

func CloseWarmEmbedded(invoke func(func())) {
	invoke(func() {
		warmWebviews.stopped = true
		warmWebviews.clear()
	})
}
