package main

import (
	"os"
	"path/filepath"
	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	"testing"
	"time"
)

func shortcutHost(t *testing.T) (*HostService, *fakeShortcutRegistry) {
	t.Helper()
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return &HostService{manager: manager}, &fakeShortcutRegistry{keys: map[string]func(){}}
}

func TestPluginRuntimeShutdownDoesNotWaitForUI(t *testing.T) {
	for _, phase := range []string{"register", "unregister", "idle"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			manager, err := kernel.New(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Close)
			source := t.TempDir()
			mf := `{"id":"example.shortcuts","name":"Shortcuts","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"open","title":"Open","uiView":"main","shortcut":"Ctrl+Shift+V"}],"uiViews":[{"id":"main","entry":"index.html","bridgeVersion":"1"}]},"permissions":[],"activationEvents":["onCommand:open"]}`
			if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(mf), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(source, "ui"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "ui", "index.html"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Install(source, false); err != nil {
				t.Fatal(err)
			}
			h := &HostService{manager: manager}
			registry := &fakeShortcutRegistry{keys: map[string]func(){}}
			queued := make(chan func(), 1)
			h.startPluginRuntimeWithDispatch(registry, func(fn func()) { queued <- fn })
			t.Cleanup(h.pluginRuntimeCancel)
			nextCall := func() func() {
				t.Helper()
				select {
				case fn := <-queued:
					return fn
				case <-time.After(3 * time.Second):
					t.Fatal("native shortcut call was not queued")
					return nil
				}
			}
			pending := nextCall()
			if len(registry.keys) != 0 {
				t.Fatal("registration ran outside the dispatcher")
			}
			if phase != "register" {
				pending()
				pending = nil
				if state := h.PluginShortcutSettings()[0].State; state.Active != "Ctrl+Shift+V" {
					t.Fatal("dispatched registration failed")
				}
				if phase == "unregister" {
					if _, err := manager.SetPluginEnabled("example.shortcuts", false); err != nil {
						t.Fatal(err)
					}
					pending = nextCall()
				}
			}
			// OnShutdown occupies the UI thread. Queued native work cannot run
			// until the runtime stops and the manager releases its directory lock.
			h.pluginRuntimeCancel()
			select {
			case <-h.pluginRuntimeDone:
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown waited for the UI thread")
			}
			manager.Close()
			next, err := kernel.New(root)
			if err != nil {
				t.Fatal("shutdown retained the directory lock", err)
			}
			defer next.Close()
			before := len(registry.keys)
			if pending != nil {
				pending()
			}
			if len(registry.keys) != before {
				t.Fatal("canceled native call ran after shutdown")
			}
			if phase != "register" && registry.keys["Ctrl+Shift+V"] == nil {
				t.Fatal("worker attempted final release instead of leaving it to Wails")
			}
		})
	}
}

func clipboardShortcut() kernel.PluginInfo {
	return kernel.PluginInfo{ID: "tools.clipboard", Name: "剪贴板", Commands: []manifest.Command{{ID: "open", Title: "剪贴板历史", UIView: "history", Shortcut: "Ctrl+Shift+V"}}}
}
func TestPluginShortcutSaveRollbackAndLifecycle(t *testing.T) {
	h, registry := shortcutHost(t)
	plugin := clipboardShortcut()
	if errs := h.syncPluginShortcuts([]kernel.PluginInfo{plugin}, registry); len(errs) != 0 {
		t.Fatal(errs)
	}
	state := h.PluginShortcutSettings()[0].State
	if state.Active != "Ctrl+Shift+V" || !state.Ready {
		t.Fatal("default not registered")
	}
	saved, err := h.SavePluginShortcut(plugin.ID, "open", "Ctrl+Alt+V", state.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if registry.keys["Ctrl+Shift+V"] != nil || registry.keys[saved.Active] == nil {
		t.Fatal("binding not switched")
	}
	if _, err = h.SavePluginShortcut(plugin.ID, "open", "Ctrl+Alt+U", state.Revision); err == nil {
		t.Fatal("stale save accepted")
	}
	registry.conflict = "Ctrl+Alt+U"
	if _, err = h.SavePluginShortcut(plugin.ID, "open", registry.conflict, saved.Revision); err == nil || registry.keys[saved.Active] == nil {
		t.Fatal("OS conflict lost old binding")
	}
	registry.conflict = ""
	entry := h.pluginShortcuts[plugin.ID+"/open"]
	original := entry.controller.preferences
	entry.controller.preferences = &fakeShortcutPreferences{fail: true}
	if _, err = h.SavePluginShortcut(plugin.ID, "open", "Ctrl+Alt+U", saved.Revision); err == nil || registry.keys[saved.Active] == nil || registry.keys["Ctrl+Alt+U"] != nil {
		t.Fatal("failed persistence lost old binding")
	}
	entry.controller.preferences = original
	plugin.Disabled = true
	h.syncPluginShortcuts([]kernel.PluginInfo{plugin}, registry)
	if len(h.PluginShortcutSettings()) != 0 || len(registry.keys) != 0 {
		t.Fatal("disabled plugin retained binding")
	}
	plugin.Disabled = false
	h.syncPluginShortcuts([]kernel.PluginInfo{plugin}, registry)
	if h.PluginShortcutSettings()[0].State.Active != saved.Active {
		t.Fatal("reenable lost override")
	}
	h.stopPluginShortcuts()
	next := &HostService{manager: h.manager}
	next.syncPluginShortcuts([]kernel.PluginInfo{plugin}, registry)
	if next.PluginShortcutSettings()[0].State.Active != saved.Active {
		t.Fatal("new host lost override")
	}
	next.syncPluginShortcuts(nil, registry)
	if len(registry.keys) != 0 {
		t.Fatal("uninstalled plugin retained binding")
	}
	retained, err := h.manager.PluginShortcutPreferences(plugin.ID, "open")
	if err != nil || retained.Values["shortcut"][0] != saved.Shortcut {
		t.Fatal("uninstall lost preferences")
	}
}
func TestHostAndPluginShortcutsShareConflictChecks(t *testing.T) {
	h, registry := shortcutHost(t)
	h.shortcuts = newShortcutControllerWithDefault(&fakeShortcutPreferences{}, registry, func() {}, nil, nil, "Ctrl+Space")
	h.shortcuts.Start()
	h.detachShortcuts = newShortcutControllerWithDefault(&fakeShortcutPreferences{}, localShortcutRegistry{}, nil, nil, nil, "Ctrl+Shift+D")
	h.detachShortcuts.Start()
	p := clipboardShortcut()
	h.syncPluginShortcuts([]kernel.PluginInfo{p}, registry)
	for _, key := range []string{"Ctrl+Space", "Ctrl+Shift+D"} {
		if _, err := h.SavePluginShortcut(p.ID, "open", key, 0); err == nil {
			t.Fatal("host shortcut conflict accepted")
		}
	}
	if _, err := h.SaveShortcut("Ctrl+Shift+V", 0); err == nil {
		t.Fatal("launcher overwrote plugin shortcut")
	}
	if _, err := h.SaveDetachShortcut("Ctrl+Shift+V", 0); err == nil {
		t.Fatal("detach overwrote plugin shortcut")
	}
	other := p
	other.ID = "tools.other"
	other.Commands = append([]manifest.Command(nil), p.Commands...)
	other.Commands[0].Shortcut = "Ctrl+Alt+U"
	h.syncPluginShortcuts([]kernel.PluginInfo{p, other}, registry)
	if _, err := h.SavePluginShortcut(p.ID, "open", "Ctrl+Alt+U", 0); err == nil {
		t.Fatal("plugin conflict accepted")
	}
	entry := h.pluginShortcuts[p.ID+"/open"]
	called, captured := 0, 0
	entry.controller.toggle = func() { called++ }
	entry.controller.captured = func(string) { captured++ }
	h.recordPluginShortcuts(true)
	entry.controller.pressed("Ctrl+Shift+V")
	if called != 0 || captured != 1 {
		t.Fatal("recording opened plugin")
	}
	h.recordPluginShortcuts(false)
	entry.controller.pressed("Ctrl+Shift+V")
	if called != 1 {
		t.Fatal("recording did not restore action")
	}
}
func TestPluginShortcutConflictRetriesWithoutStaleError(t *testing.T) {
	h, registry := shortcutHost(t)
	registry.conflict = "Ctrl+Shift+V"
	p := clipboardShortcut()
	if errs := h.syncPluginShortcuts([]kernel.PluginInfo{p}, registry); len(errs) != 1 {
		t.Fatal("missing conflict diagnosis")
	}
	registry.conflict = ""
	if errs := h.syncPluginShortcuts([]kernel.PluginInfo{p}, registry); len(errs) != 0 {
		t.Fatal("conflict not recovered", errs)
	}
	if state := h.PluginShortcutSettings()[0].State; state.Active != "Ctrl+Shift+V" || state.Error != "" {
		t.Fatal("recovered binding kept error")
	}
}
