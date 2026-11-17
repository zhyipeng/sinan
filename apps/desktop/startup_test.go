package main

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
	"testing"
)

type fakeStartup struct {
	enabled         bool
	fail            bool
	readFail        bool
	requireApproval bool
	enables         int
	disables        int
	options         application.AutostartOptions
	readError       error
	writeError      error
}

func (f *fakeStartup) IsEnabled() (bool, error) {
	if f.readError != nil {
		return false, f.readError
	}
	if f.readFail {
		return false, errors.New("underlying read details")
	}
	return f.enabled, nil
}
func (f *fakeStartup) EnableWithOptions(options application.AutostartOptions) error {
	f.enables++
	f.options = options
	if f.requireApproval {
		return nil
	}
	return f.SetEnabled(true)
}
func (f *fakeStartup) Disable() error { f.disables++; return f.SetEnabled(false) }
func (f *fakeStartup) SetEnabled(value bool) error {
	if f.writeError != nil {
		return f.writeError
	}
	if f.fail {
		return errors.New("无法保存开机自启配置")
	}
	f.enabled = value
	return nil
}

func TestDesktopWailsAutostartOptionsAndLegacyRegistration(t *testing.T) {
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := &fakeStartup{enabled: true}
	h := &HostService{manager: m, startup: r}
	if _, err = h.SaveAutoStart(true); err != nil || r.enables != 0 {
		t.Fatal("legacy registration must not be enabled a second time")
	}
	state, err := h.SaveAutoStart(false)
	if err != nil || state.AutoStart || r.disables != 1 {
		t.Fatalf("disable: %+v %v", state, err)
	}
	state, err = h.SaveAutoStart(true)
	if err != nil || !state.AutoStart || r.enables != 1 {
		t.Fatalf("enable: %+v %v", state, err)
	}
	if r.options.Identifier != autostartIdentifier || len(r.options.Arguments) != 0 {
		t.Fatal("unexpected autostart identity or startup arguments")
	}
	if _, err = h.SaveAutoStart(true); err != nil || r.enables != 1 {
		t.Fatal("repeated enable registered a duplicate")
	}
}

func TestDesktopWailsAutostartApprovalAndErrors(t *testing.T) {
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := &fakeStartup{requireApproval: true}
	h := &HostService{manager: m, startup: r}
	if state, err := h.SaveAutoStart(true); err == nil || state.AutoStart {
		t.Fatal("pending approval reported as enabled")
	}
	r.readFail = true
	if _, err = h.SaveAutoStart(true); err == nil || err.Error() != "无法读取开机自启配置" {
		t.Fatal("read failure ignored or raw details returned")
	}
	if r.enables != 1 {
		t.Fatal("read failure attempted registration")
	}
	state, err := h.StartupSettings()
	if err != nil || state.AutoStartError != "无法读取开机自启配置" {
		t.Fatalf("read state: %+v %v", state, err)
	}
	r.readFail = false
	r.requireApproval = false
	r.writeError = application.ErrAutostartNotSupported
	if _, err = h.SaveAutoStart(true); err == nil || err.Error() != "当前平台不支持开机自启" {
		t.Fatal("unsupported platform not reported")
	}
	r.readError = application.ErrAutostartNotSupported
	state, err = h.SaveSilentStartup(true, 0)
	if err != nil || !state.Silent || state.AutoStartError != "当前平台不支持开机自启" {
		t.Fatalf("silent must remain independent: %+v %v", state, err)
	}
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
