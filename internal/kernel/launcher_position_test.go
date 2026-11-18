package kernel

import (
	"path/filepath"
	"testing"
)

func TestLauncherPositionPreferencesPersistAndValidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	state, err := m.LauncherPositionPreferences()
	if err != nil || state.TopOffsetPercent != DefaultLauncherTopOffset || state.Revision != 0 {
		t.Fatalf("defaults: %+v %v", state, err)
	}
	for _, invalid := range []int{-1, 51} {
		if _, err := m.SaveLauncherPositionPreferences(invalid, 0); err == nil {
			t.Errorf("accepted offset %d", invalid)
		}
	}
	state, err = m.SaveLauncherPositionPreferences(0, 0)
	if err != nil || state.TopOffsetPercent != 0 || state.Revision != 1 {
		t.Fatalf("save zero: %+v %v", state, err)
	}
	if _, err := m.SaveLauncherPositionPreferences(50, 0); err == nil {
		t.Fatal("accepted stale revision")
	}
	m.Close()

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err = reopened.LauncherPositionPreferences()
	if err != nil || state.TopOffsetPercent != 0 || state.Revision != 1 {
		t.Fatalf("reload zero: %+v %v", state, err)
	}
	state, err = reopened.SaveLauncherPositionPreferences(50, state.Revision)
	if err != nil || state.TopOffsetPercent != 50 || state.Revision != 2 {
		t.Fatalf("save maximum: %+v %v", state, err)
	}
}

func TestLauncherPositionPreferencesPreserveInvalidRecord(t *testing.T) {
	m := manager(t)
	if _, err := m.settings.Update(launcherPositionNamespace, 0, map[string][]string{"topOffsetPercent": {"1.5"}}); err != nil {
		t.Fatal(err)
	}
	state, err := m.LauncherPositionPreferences()
	if err != nil || state.Error == "" || state.TopOffsetPercent != DefaultLauncherTopOffset {
		t.Fatalf("invalid preference fallback: %+v %v", state, err)
	}
	if _, err := m.SaveLauncherPositionPreferences(15, state.Revision); err == nil {
		t.Fatal("overwrote invalid record")
	}
	if got := m.settings.Get(launcherPositionNamespace); got.Revision != state.Revision {
		t.Fatalf("changed invalid record: %+v", got)
	}
}
