package main

import (
	"errors"
	"sync"
	"testing"

	"sinan/internal/kernel"
	"sinan/internal/storage"
)

type fakeShortcutPreferences struct {
	snapshot storage.SettingsSnapshot
	fail     bool
}

func (p *fakeShortcutPreferences) LauncherPreferences() (storage.SettingsSnapshot, error) {
	return p.snapshot, nil
}
func (p *fakeShortcutPreferences) SaveLauncherPreferences(revision uint64, value string) (storage.SettingsSnapshot, error) {
	if p.fail {
		return storage.SettingsSnapshot{}, errors.New("fixture storage failure")
	}
	p.snapshot = storage.SettingsSnapshot{Revision: revision + 1, Values: map[string][]string{"shortcut": {value}}}
	return p.snapshot, nil
}

type fakeShortcutRegistry struct {
	mu          sync.Mutex
	keys        map[string]func()
	conflict    string
	failRelease string
}

func (r *fakeShortcutRegistry) Register(key string, callback func()) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key == r.conflict {
		return errors.New("fixture conflict")
	}
	if r.keys[key] != nil {
		return errors.New("already registered")
	}
	r.keys[key] = callback
	return nil
}
func (r *fakeShortcutRegistry) Unregister(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, key)
	if key == r.failRelease {
		return errors.New("fixture release failure")
	}
	return nil
}
func shortcutFixture() (*shortcutController, *fakeShortcutRegistry, *fakeShortcutPreferences) {
	registry := &fakeShortcutRegistry{keys: map[string]func(){}}
	prefs := &fakeShortcutPreferences{snapshot: storage.SettingsSnapshot{Values: map[string][]string{}}}
	c := newShortcutController(prefs, registry, func() {}, nil, nil)
	c.Start()
	return c, registry, prefs
}
func TestShortcutValidationAndNormalization(t *testing.T) {
	for _, invalid := range []string{"A", "Shift+A", "Ctrl+Ctrl+A", "Ctrl+Escape", "Alt+F99", "Ctrl+Unknown"} {
		if _, err := normalizeShortcut(invalid); err == nil {
			t.Fatal("invalid shortcut accepted", invalid)
		}
	}
	got, err := normalizeShortcut("shift+ctrl+k")
	if err != nil || got != "Ctrl+Shift+K" {
		t.Fatal("shortcut not canonical", got, err)
	}
}
func TestShortcutSwitchConflictAndPersistenceRollback(t *testing.T) {
	c, r, p := shortcutFixture()
	initial := c.State()
	r.conflict = "Ctrl+Alt+K"
	if _, err := c.Save(r.conflict, 0); err == nil || c.State().Active != initial.Active || p.snapshot.Revision != 0 {
		t.Fatal("conflict changed old shortcut")
	}
	r.conflict = ""
	state, err := c.Save("Ctrl+Alt+K", 0)
	if err != nil || state.Active != "Ctrl+Alt+K" || state.Revision != 1 || r.keys[initial.Active] != nil {
		t.Fatal("switch failed", err)
	}
	p.fail = true
	if _, err := c.Save("Ctrl+Shift+K", 1); err == nil || c.State().Active != "Ctrl+Alt+K" || r.keys["Ctrl+Alt+K"] == nil || r.keys["Ctrl+Shift+K"] != nil {
		t.Fatal("persistence failure lost old registration")
	}
	p.fail = false
	if _, err := c.Save("Ctrl+Shift+K", 0); err == nil {
		t.Fatal("stale save accepted")
	}
	// A fresh controller loads the persisted preference rather than the default.
	other := newShortcutController(p, &fakeShortcutRegistry{keys: map[string]func(){}}, func() {}, nil, nil)
	other.Start()
	if other.State().Active != "Ctrl+Alt+K" {
		t.Fatal("saved preference not loaded")
	}
}
func TestShortcutReleaseFailureAndRecording(t *testing.T) {
	c, r, _ := shortcutFixture()
	old := c.State().Active
	r.failRelease = old
	if _, err := c.Save("Ctrl+Alt+K", 0); err == nil || r.keys[old] == nil || r.keys["Ctrl+Alt+K"] != nil {
		t.Fatal("release failure did not restore old key")
	}
	captured := make(chan string, 1)
	toggled := make(chan struct{}, 1)
	c.captured = func(key string) { captured <- key }
	c.toggle = func() { toggled <- struct{}{} }
	c.Record(true)
	c.pressed(old)
	select {
	case key := <-captured:
		if key != old {
			t.Fatal("wrong recording")
		}
	default:
		t.Fatal("recording toggled launcher")
	}
	c.Record(false)
	c.pressed(old)
	select {
	case <-toggled:
	default:
		t.Fatal("recording cancellation did not restore toggle")
	}
	c.pressed("Ctrl+Alt+K")
	select {
	case <-toggled:
		t.Fatal("inactive shortcut toggled launcher")
	default:
	}
}

func TestDetachShortcutLocalSaveConflictAndPersistence(t *testing.T) {
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	local := newShortcutControllerWithDefault(windowPreferences{manager}, localShortcutRegistry{}, nil, nil, nil, defaultDetachShortcut())
	local.Start()
	launcher, _, _ := shortcutFixture()
	h := &HostService{shortcuts: launcher, detachShortcuts: local}
	if _, err := h.SaveDetachShortcut(launcher.State().Shortcut, 0); err == nil {
		t.Fatal("same launcher/detach key accepted")
	}
	before := local.State()
	saved, err := h.SaveDetachShortcut("Ctrl+Alt+F19", before.Revision)
	if err != nil || saved.Active != "Ctrl+Alt+F19" {
		t.Fatal("local save failed", err)
	}
	if _, err := h.SaveShortcut("Ctrl+Alt+F19", launcher.State().Revision); err == nil {
		t.Fatal("launcher overwrote detach binding")
	}
	if _, err := h.SaveDetachShortcut("Ctrl+Alt+K", before.Revision); err == nil {
		t.Fatal("stale window overwrote shortcut")
	}
	if _, err := h.SaveDetachShortcut("Shift+D", saved.Revision); err == nil {
		t.Fatal("invalid chord accepted")
	}
	loaded := newShortcutControllerWithDefault(windowPreferences{manager}, localShortcutRegistry{}, nil, nil, nil, defaultDetachShortcut())
	loaded.Start()
	if loaded.State().Active != saved.Active {
		t.Fatal("binding was not persisted")
	}
	result, err := h.SaveDetachShortcut(saved.Default, saved.Revision)
	if err != nil || result.Active != saved.Default {
		t.Fatal("default restore failed", err)
	}
}
