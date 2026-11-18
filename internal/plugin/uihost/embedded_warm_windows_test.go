//go:build windows && (amd64 || arm64) && !native_webview2loader

package uihost

import (
	"testing"

	"sinan/internal/plugin/uiview"
)

func TestWebviewWarmSlotClaimsOnlyOnceForMatchingWindow(t *testing.T) {
	view := &Embedded{creating: true}
	slot := webviewWarmSlot{hwnd: 1, view: view}

	if slot.take(2) != nil || slot.view != view {
		t.Fatal("a different window must not take the pending controller")
	}
	if slot.take(1) != view || slot.take(1) != nil {
		t.Fatal("the pending controller must transfer exactly once")
	}
	slot.clear()
	if view.closed.Load() {
		t.Fatal("clearing the slot must not close a transferred controller")
	}
}

func TestWebviewWarmSlotRejectsUnavailableController(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		closed  bool
		bound   bool
		stopped bool
	}{
		{name: "failed", status: -1},
		{name: "closed", closed: true},
		{name: "already bound", bound: true},
		{name: "shutdown", stopped: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := &Embedded{status: test.status}
			view.closed.Store(test.closed)
			if test.bound {
				view.session = &uiview.Session{}
			}
			slot := webviewWarmSlot{hwnd: 1, view: view, stopped: test.stopped}
			if slot.take(1) != nil {
				t.Fatal("an unavailable controller must not transfer")
			}
		})
	}
}

func TestWebviewWarmSlotClearsUnclaimedController(t *testing.T) {
	view := &Embedded{creating: true, done: make(chan struct{})}
	slot := webviewWarmSlot{hwnd: 1, view: view}

	slot.clear()
	slot.clear()
	select {
	case <-view.Done():
	default:
		t.Fatal("clearing the slot must close its unclaimed controller")
	}
	if slot.take(1) != nil || !view.finishCreation() {
		t.Fatal("a late callback must observe the already closed controller")
	}
}
