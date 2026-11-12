package main

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
)

func TestTrustedViewRequiresInstalledBundledDeclaration(t *testing.T) {
	snapshot := kernel.Snapshot{Plugins: []kernel.PluginInfo{
		{ID: "system.settings", Commands: []manifest.Command{{ID: "open", Title: "设置", View: "settings"}}},
		{ID: "system.plugin-manager", Commands: []manifest.Command{{ID: "open", Title: "插件管理", View: "manager"}}},
		{ID: "system.plugin-store", Commands: []manifest.Command{{ID: "open", Title: "插件商店", View: "store"}}},
		{ID: "third.party", Commands: []manifest.Command{{ID: "open", View: "settings"}}},
	}}
	for _, id := range []string{"system.settings", "system.plugin-manager", "system.plugin-store"} {
		title, key, err := trustedView(snapshot, id, "open")
		if err != nil || title == "" || key == "" {
			t.Fatal("bundled view rejected", err)
		}
	}
	for _, target := range [][2]string{{"third.party", "open"}, {"missing", "open"}, {"system.settings", "missing"}} {
		if _, _, err := trustedView(snapshot, target[0], target[1]); err == nil {
			t.Fatal("unregistered view accepted")
		}
	}
	snapshot.Plugins[0].Disabled = true
	if _, _, err := trustedView(snapshot, "system.settings", "open"); err == nil {
		t.Fatal("disabled view accepted")
	}
	snapshot.Plugins[1].Commands[0].View = "unknown"
	if _, _, err := trustedView(snapshot, "system.plugin-manager", "open"); err == nil {
		t.Fatal("unknown component accepted")
	}
}

func TestTrustedWindowCallerFollowsRoleAndRejectsStaleWindow(t *testing.T) {
	original := &application.WebviewWindow{}
	replacement := &application.WebviewWindow{}
	unknown := &application.WebviewWindow{}
	l := &launcher{window: original, detached: map[uint]trustedWindow{}}
	ctx := context.WithValue(context.Background(), application.WindowKey, original)
	if l.caller(ctx) != original || l.caller(context.Background()) != nil {
		t.Fatal("initial caller mismatch")
	}
	l.window = replacement
	l.detached[original.ID()] = trustedWindow{window: original, key: "system.settings/settings"}
	if l.caller(ctx) != original || l.isMain(original) {
		t.Fatal("original did not become independent")
	}
	if l.caller(context.WithValue(context.Background(), application.WindowKey, unknown)) != nil {
		t.Fatal("unknown window accepted")
	}
	l.forget(original)
	if l.caller(ctx) != nil {
		t.Fatal("closed window retained bridge ownership")
	}
}

func TestRecordingCancellationIsOwnedByItsWindow(t *testing.T) {
	shortcut, _, _ := shortcutFixture()
	original := &application.WebviewWindow{}
	replacement := &application.WebviewWindow{}
	l := &launcher{window: replacement, detached: map[uint]trustedWindow{
		original.ID(): {window: original, key: "system.settings/settings"},
	}}
	h := &HostService{launcher: l, shortcuts: shortcut, recordingWindow: replacement}
	if err := shortcut.Record(true); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), application.WindowKey, original)
	if err := h.RecordShortcut(ctx, false); err != nil {
		t.Fatal(err)
	}
	h.stopRecording(original)
	if shortcut.recordingUntil.IsZero() {
		t.Fatal("unrelated window cancelled recording")
	}
	h.stopRecording(replacement)
	if !shortcut.recordingUntil.IsZero() || h.recordingWindow != nil {
		t.Fatal("owner blur did not cancel recording")
	}
}

func TestDetachRejectsMissingOrDetachedCallerBeforeWindowCreation(t *testing.T) {
	original := &application.WebviewWindow{}
	main := &application.WebviewWindow{}
	l := &launcher{window: main, newWindow: func() *application.WebviewWindow {
		t.Fatal("invalid caller created a window")
		return nil
	}}
	for _, caller := range []*application.WebviewWindow{nil, original} {
		if err := l.detach(caller, "设置", "system.settings/settings"); err == nil {
			t.Fatal("invalid caller detached")
		}
	}
}

func TestDetachedWindowCannotCancelMainSearch(t *testing.T) {
	detached := &application.WebviewWindow{}
	main := &application.WebviewWindow{}
	l := &launcher{window: main, detached: map[uint]trustedWindow{
		detached.ID(): {window: detached, key: "system.settings/settings"},
	}}
	cancelled := false
	h := &HostService{launcher: l, searchSequence: 99, searchCancel: func() { cancelled = true }}
	h.CancelSearch(context.WithValue(context.Background(), application.WindowKey, detached), 99)
	if cancelled {
		t.Fatal("detached window cancelled main search")
	}
	h.CancelSearch(context.WithValue(context.Background(), application.WindowKey, main), 99)
	if !cancelled {
		t.Fatal("main window could not cancel its search")
	}
}
