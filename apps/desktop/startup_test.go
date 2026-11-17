package main

import (
	"errors"
	"sinan/internal/kernel"
	"testing"
)

type fakeStartup struct {
	enabled bool
	fail    bool
}

func (f *fakeStartup) Enabled() (bool, error) { return f.enabled, nil }
func (f *fakeStartup) SetEnabled(value bool) error {
	if f.fail {
		return errors.New("无法保存开机自启配置")
	}
	f.enabled = value
	return nil
}
func TestDesktopStartupIndependentSwitches(t *testing.T) {
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := &fakeStartup{}
	h := &HostService{manager: m, startup: r}
	state, err := h.SaveSilentStartup(true, 0)
	if err != nil || !state.Silent || state.AutoStart {
		t.Fatalf("silent: %+v %v", state, err)
	}
	state, err = h.SaveAutoStart(true)
	if err != nil || !state.Silent || !state.AutoStart {
		t.Fatalf("auto: %+v %v", state, err)
	}
	r.fail = true
	if _, err = h.SaveAutoStart(false); err == nil {
		t.Fatal("registration failure ignored")
	}
	state, _ = h.StartupSettings()
	if !state.AutoStart || !state.Silent {
		t.Fatal("failed change altered state")
	}
	h.startup = nil
	h.startupError = "开发和验收实例不支持开机自启"
	if _, err = h.SaveAutoStart(true); err == nil {
		t.Fatal("development registration allowed")
	}
	state, err = h.SaveSilentStartup(false, state.Revision)
	if err != nil || state.Silent || state.AutoStartError == "" {
		t.Fatalf("independent fallback: %+v %v", state, err)
	}
}
