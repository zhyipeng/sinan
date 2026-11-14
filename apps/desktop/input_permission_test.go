package main

import (
	"errors"
	"fmt"
	"sinan/internal/platform"
	"testing"
)

func TestAccessibilityGuideOnlyForMissingPermission(t *testing.T) {
	var guide accessibilityGuide
	var queued []func()
	schedule := func(fn func()) { queued = append(queued, fn) }
	shown := 0
	show := func() { shown++ }
	valid := func() bool { return true }
	guide.request(errors.New("target closed"), valid, schedule, show)
	if len(queued) != 0 {
		t.Fatal("unrelated error prompted for consent")
	}
	guide.request(fmt.Errorf("input: %w", platform.ErrAccessibilityPermission), valid, schedule, show)
	guide.request(platform.ErrAccessibilityPermission, valid, schedule, show)
	if len(queued) != 1 {
		t.Fatal("duplicate permission dialogs queued")
	}
	queued[0]()
	if shown != 1 {
		t.Fatal("missing permission did not show guide")
	}
	guide.request(platform.ErrAccessibilityPermission, valid, schedule, show)
	if len(queued) != 2 {
		t.Fatal("dismissed guide could not be retried")
	}
}
func TestAccessibilityGuideSkipsRevokedView(t *testing.T) {
	var guide accessibilityGuide
	var queued func()
	active := true
	shown := false
	guide.request(platform.ErrAccessibilityPermission, func() bool { return active }, func(fn func()) { queued = fn }, func() { shown = true })
	active = false
	queued()
	if shown || guide.pending.Load() {
		t.Fatal("revoked view retained permission dialog")
	}
}
