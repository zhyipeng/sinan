package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/plugin/uiview"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestWindowCommandHidesBeforeExecutionAndRejectsReplacement(t *testing.T) {
	var order []string
	result, err := runWindowCommand(context.Background(), func() bool { return true }, func() { order = append(order, "hide") }, func() (json.RawMessage, error) {
		order = append(order, "capture")
		return json.RawMessage(`{"done":true}`), nil
	}, func() { order = append(order, "restore") })
	if err != nil || string(result) != `{"done":true}` || len(order) != 3 || order[0] != "hide" || order[1] != "capture" || order[2] != "restore" {
		t.Fatal(order, err)
	}
	active := true
	executed := false
	_, err = runWindowCommand(context.Background(), func() bool { return active }, func() { active = false }, func() (json.RawMessage, error) { executed = true; return nil, nil }, func() {})
	if err == nil || executed {
		t.Fatal("replaced view still executed native command")
	}
}
func TestWindowCommandCancelAndNativeFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executed := false
	started := time.Now()
	_, err := runWindowCommand(ctx, func() bool { return true }, cancel, func() (json.RawMessage, error) { executed = true; return nil, nil }, func() {})
	if !errors.Is(err, context.Canceled) || executed || time.Since(started) > time.Second {
		t.Fatal("cancellation did not stop command", err)
	}
	expected := errors.New("native unavailable")
	_, err = runWindowCommand(context.Background(), func() bool { return true }, func() {}, func() (json.RawMessage, error) { return nil, expected }, func() {})
	if !errors.Is(err, expected) {
		t.Fatal("native error swallowed")
	}
}

func TestWindowCommandRestoreBelongsToCurrentLiveSession(t *testing.T) {
	for _, action := range []string{"complete", "cancel", "failure", "replace", "revoke"} {
		t.Run(action, func(t *testing.T) {
			bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.window", "index.html")
			if err != nil {
				t.Fatal(err)
			}
			session, err := uiview.NewSession(uiview.Binding{PluginID: "example.window", ViewID: "main", BridgeVersion: "2", Fingerprint: strings.Repeat("a", 64)}, bundle)
			if err != nil {
				t.Fatal(err)
			}
			window := &application.WebviewWindow{}
			entry := &embeddedEntry{window: window, session: session, executing: true}
			host := &HostService{embedded: &embeddedViews{entries: map[*application.WebviewWindow]*embeddedEntry{window: entry}}}
			if action == "replace" {
				host.embedded.entries[window] = &embeddedEntry{window: window}
			}
			if action == "revoke" {
				session.Revoke()
			}
			shown := 0
			host.finishWindowCommand(entry, func() { shown++ })
			expected := 1
			if action == "replace" || action == "revoke" {
				expected = 0
			}
			if shown != expected || entry.executing {
				t.Fatal("wrong restoration", shown, entry.executing)
			}
		})
	}
}

func TestWindowCommandDismissUsesResultFlagOnly(t *testing.T) {
	if !windowCommandDismissed(nil, errors.New("transport failed"), true) || windowCommandDismissed(nil, errors.New("transport failed"), false) {
		t.Fatal("hidden launch failure opened editor or changed ordinary restoration")
	}
	for _, value := range []string{`{"text":"cancelled","dismiss":true}`, `{"dismiss":true}`} {
		if !windowCommandDismissed(json.RawMessage(value), nil, false) {
			t.Fatal("dismiss ignored")
		}
	}
	for _, value := range []string{`{"text":"{\"dismiss\":true}"}`, `{"text":"image","dismiss":false}`, `invalid`} {
		if windowCommandDismissed(json.RawMessage(value), nil, false) {
			t.Fatal("business data caused dismissal")
		}
	}
}
func TestWindowCommandDismissReleasesOnlyOriginalHiddenView(t *testing.T) {
	for _, replace := range []bool{false, true} {
		bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.window", "index.html")
		if err != nil {
			t.Fatal(err)
		}
		session, err := uiview.NewSession(uiview.Binding{PluginID: "example.window", ViewID: "main", BridgeVersion: "2", Fingerprint: strings.Repeat("a", 64)}, bundle)
		if err != nil {
			t.Fatal(err)
		}
		window := &application.WebviewWindow{}
		view := &fakeEmbeddedSurface{}
		entry := &embeddedEntry{window: window, session: session, view: view, executing: true}
		host := &HostService{embedded: &embeddedViews{entries: map[*application.WebviewWindow]*embeddedEntry{window: entry}, sequence: map[*application.WebviewWindow]uint64{window: 2}}}
		if replace {
			host.embedded.entries[window] = &embeddedEntry{window: window}
		}
		discarded := host.discardWindowCommand(entry)
		if replace {
			if discarded || view.closed != 0 || !session.Active() || host.embedded.sequence[window] != 2 {
				t.Fatal("dismiss crossed session boundary")
			}
		} else if !discarded || view.closed != 1 || session.Active() || host.embedded.entries[window] != nil || host.embedded.sequence[window] != 3 {
			t.Fatal("dismiss left hidden editor active")
		}
	}
}
