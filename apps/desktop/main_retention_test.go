package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"testing"
	"time"
)

func TestDesktopMainRetentionHiddenTimer(t *testing.T) {
	// An uninitialised Wails value has no native implementation or event loop.
	l := &launcher{window: &application.WebviewWindow{}}
	defer l.closeRetention()
	l.retention.preferences = kernel.RetentionPreferences{ClearInputSeconds: 1, PluginUISeconds: 10}
	l.retention.hidden(time.Now().Add(-time.Second + 20*time.Millisecond))
	l.scheduleInputClearLocked()
	deadline := time.Now().Add(time.Second)
	for {
		l.mu.Lock()
		cleared := l.retention.inputCleared
		decision := l.retention.decision(time.Now())
		l.mu.Unlock()
		if cleared {
			if decision.PreserveInput || !decision.PreserveView {
				t.Fatal("timer expiry interfered with UI retention")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hidden timer did not clear input")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestDesktopMainRetentionIndependentDeadlines(t *testing.T) {
	start := time.Now()
	for _, test := range []struct {
		input, plugin, elapsed int
		keepInput, keepView    bool
	}{
		{0, 0, 0, false, false}, {10, 60, 5, true, true}, {10, 60, 10, false, true}, {10, 60, 60, false, false},
		{60, 10, 10, true, false}, {60, 0, 5, true, false}, {0, 60, 5, false, true},
	} {
		r := mainRetention{preferences: kernel.RetentionPreferences{ClearInputSeconds: test.input, PluginUISeconds: test.plugin}}
		r.hidden(start)
		state := r.decision(start.Add(time.Duration(test.elapsed) * time.Second))
		if state.PreserveInput != test.keepInput || state.PreserveView != test.keepView {
			t.Fatalf("%+v: %+v", test, state)
		}
		// Redundant hide must not reset the hidden timestamp or revive expired UI.
		if r.hidden(start.Add(100*time.Second)) || !r.hiddenAt.Equal(start) {
			t.Fatal("duplicate hide extended deadlines")
		}
	}
}

func TestDesktopMainRetentionReopenDetachAndStaleTimer(t *testing.T) {
	start := time.Now()
	r := mainRetention{preferences: kernel.RetentionPreferences{ClearInputSeconds: 10, PluginUISeconds: 30}}
	r.hidden(start)
	oldEpoch := r.epoch
	r.shown()
	if r.expireInput(oldEpoch) {
		t.Fatal("hidden timer cleared a reopened window")
	}
	if state := r.decision(start); state.PreserveView || state.PreserveInput {
		t.Fatal("new window inherited an old session")
	}
	r.hidden(start.Add(time.Second))
	if r.expireInput(oldEpoch) {
		t.Fatal("old hide cleared a new hide session")
	}
	if !r.expireInput(r.epoch) {
		t.Fatal("current timer did not expire")
	}
	state := r.decision(start.Add(2 * time.Second))
	if state.PreserveInput || !state.PreserveView {
		t.Fatal("input expiry destroyed retained UI")
	}
	// Increasing a duration after clearing cannot recreate deleted input.
	r.preferences.ClearInputSeconds = 100
	if r.decision(start.Add(3 * time.Second)).PreserveInput {
		t.Fatal("deleted input was revived")
	}
	r.closed = true
	if r.hidden(start) || r.expireInput(r.epoch) {
		t.Fatal("closed lifecycle accepted work")
	}
}

func TestDesktopMainRetentionRescheduleInvalidatesWaitingCallback(t *testing.T) {
	l := &launcher{}
	l.retention.preferences.ClearInputSeconds = 30
	l.retention.hidden(time.Now())
	l.scheduleInputClearLocked()
	first := l.retention.epoch
	l.retention.preferences.ClearInputSeconds = 60
	l.scheduleInputClearLocked()
	if l.retention.expireInput(first) {
		t.Fatal("superseded timer cleared input before new deadline")
	}
	l.closeRetention()
	if l.retention.timer != nil || l.retention.expireInput(l.retention.epoch) {
		t.Fatal("shutdown left timer active")
	}
}

func TestDesktopRetentionSaveUpdatesRuntimeOnlyAfterPersistence(t *testing.T) {
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	l := &launcher{}
	h := &HostService{manager: m, launcher: l}
	state, err := h.SaveRetentionSettings(10, 60, 0)
	// No native window or event loop is needed for host-owned settings.
	if err != nil || state.ClearInputSeconds != 10 || l.retention.preferences.PluginUISeconds != 60 {
		t.Fatalf("save: %+v %v", state, err)
	}
	if _, err = h.SaveRetentionSettings(20, 30, 0); err == nil {
		t.Fatal("stale save accepted")
	}
	if l.retention.preferences.ClearInputSeconds != 10 || l.retention.preferences.PluginUISeconds != 60 {
		t.Fatal("failed save changed runtime policy")
	}
}
