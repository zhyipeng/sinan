package main

import (
	"errors"
	"os"
	"path/filepath"
	"sinan/internal/kernel"
	"testing"
)

func TestDesktopWindowOpacityFailureAndRollback(t *testing.T) {
	root := t.TempDir()
	m, err := kernel.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	actual := 100
	fail := false
	h := &HostService{opacity: &windowOpacityController{manager: m, apply: func(value int) error {
		if fail {
			return errors.New("原生能力不可用")
		}
		actual = value
		return nil
	}}}
	state, err := h.SaveWindowOpacity(75, 0)
	if err != nil || state.Percent != 75 || actual != 75 {
		t.Fatalf("save: %+v %v actual=%d", state, err, actual)
	}
	if _, err = h.SaveWindowOpacity(60, 0); err == nil || actual != 75 {
		t.Fatal("stale save changed native window")
	}
	if _, err = h.SaveWindowOpacity(0, 1); err == nil || actual != 75 {
		t.Fatal("invalid save changed native window")
	}
	fail = true
	if _, err = h.SaveWindowOpacity(60, 1); err == nil {
		t.Fatal("native failure ignored")
	}
	state, _ = m.WindowOpacityPreferences()
	if state.Percent != 75 || state.Revision != 1 {
		t.Fatal("native failure changed preferences")
	}
	state, err = h.WindowOpacitySettings()
	if err != nil || state.Error == "" || state.Percent != 75 {
		t.Fatalf("missing capability error: %+v %v", state, err)
	}
	fail = false
	path := filepath.Join(root, ".sinan-state", "settings.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = h.SaveWindowOpacity(50, 1); err == nil {
		t.Fatal("storage failure ignored")
	}
	if actual != 75 {
		t.Fatal("native opacity not rolled back")
	}
	state, _ = m.WindowOpacityPreferences()
	if state.Percent != 75 || state.Revision != 1 {
		t.Fatal("storage failure changed preferences")
	}
}
