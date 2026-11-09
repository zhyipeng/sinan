package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
)

func unapprovedUI(t *testing.T, m *Manager) string {
	t.Helper()
	src := uiPackage(t, true, "0.1.0")
	mf, _ := manifest.Load(src)
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	mf.Contributes.UIViews[0].Commands = []string{"echo", "hang"}
	mf.Contributes.UIViews[0].Permissions = []string{"storage"}
	writeManifest(t, src, mf)
	if _, err := m.Install(src, true); err != nil {
		t.Fatal(err)
	}
	return src
}
func TestUIPermissionsDefaultDenyAndPartialGrant(t *testing.T) {
	m := manager(t)
	unapprovedUI(t, m)
	s := openUI(t, m, "example.native")
	s.Message([]byte(`{"kind":"ready","version":2}`))
	for _, request := range []string{`{"method":"storage.get"}`, `{"method":"command.execute","commandId":"echo","input":"fixture"}`} {
		if _, err := s.Call(context.Background(), []byte(request)); err == nil {
			t.Fatal("declaration implicitly authorized")
		}
	}
	state, err := m.UIPermissions()
	if err != nil || len(state.Plugins) != 1 || state.Plugins[0].Reviewed {
		t.Fatal("bad initial policy", err)
	}
	plugin := state.Plugins[0]
	if _, err = m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision, []string{"main/command/echo"}); err != nil {
		t.Fatal(err)
	}
	if s.Active() {
		t.Fatal("grant change did not retire old session")
	}
	s = openUI(t, m, plugin.ID)
	s.Message([]byte(`{"kind":"ready","version":2}`))
	callUI(t, s, `{"method":"command.execute","commandId":"echo","input":"fixture"}`)
	if _, err = s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("partial grant authorized storage")
	}
	if _, err = s.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"hang"}`)); err == nil {
		t.Fatal("partial grant authorized other command")
	}
	state, _ = m.UIPermissions()
	if _, err = m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision, []string{}); err != nil {
		t.Fatal(err)
	}
	if s.Active() {
		t.Fatal("revocation did not retire session")
	}
	current := openUI(t, m, plugin.ID)
	current.Message([]byte(`{"kind":"ready","version":2}`))
	if _, err = current.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"echo"}`)); err == nil {
		t.Fatal("revoked command executed")
	}
}
func TestUIPermissionsReviewConflictsAndFailureRetention(t *testing.T) {
	m := manager(t)
	s, _ := callableUI(t, m)
	state, _ := m.UIPermissions()
	plugin := state.Plugins[0]
	for _, caps := range [][]string{{"main/command/crash"}, {"main/storage", "main/storage"}, {"other/storage"}} {
		if _, err := m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision, caps); err == nil {
			t.Fatal("undeclared or duplicate permission accepted")
		}
	}
	if _, err := m.SaveUIPermissions(plugin.ID, "stale", state.Revision, []string{}); err == nil {
		t.Fatal("stale package review accepted")
	}
	if _, err := m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision-1, []string{}); err == nil {
		t.Fatal("stale revision accepted")
	}
	// Block persistence using a directory at the core configuration path.
	config := filepath.Join(m.root, ".sinan-state", "settings.json")
	if err := os.Rename(config, config+".previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(config, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision, []string{}); err == nil {
		t.Fatal("failed policy write reported success")
	}
	if !s.Active() {
		t.Fatal("failed grant write revoked session")
	}
	callUI(t, s, `{"method":"storage.get"}`)
}
func TestUIPermissionsPersistAndRequireReviewForChangedPackage(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { m.Close() }()
	src := unapprovedUI(t, m)
	grantAllUI(t, m, "example.native")
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	s := openUI(t, m, "example.native")
	s.Message([]byte(`{"kind":"ready","version":2}`))
	callUI(t, s, `{"method":"storage.get"}`)
	state, _ := m.UIPermissions()
	previous := state.Plugins[0]
	mf, _ := manifest.Load(src)
	mf.Version = "0.2.0"
	writeManifest(t, src, mf)
	if _, err = m.Upgrade(src, true); err != nil {
		t.Fatal(err)
	}
	state, _ = m.UIPermissions()
	if !state.Plugins[0].Stale {
		t.Fatal("updated package was not marked stale")
	}
	next := openUI(t, m, "example.native")
	next.Message([]byte(`{"kind":"ready","version":2}`))
	if _, err = next.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("new package inherited authorization")
	}
	if _, err = m.SaveUIPermissions(previous.ID, previous.Fingerprint, state.Revision, []string{"main/storage"}); err == nil {
		t.Fatal("old package review granted new package")
	}
	grantAllUI(t, m, "example.native")
}
func TestUIPermissionsCorruptionFailsClosedWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { m.Close() }()
	unapprovedUI(t, m)
	if _, err = m.settings.Update(uiPermissionsNamespace, 0, map[string][]string{"example.native": {"invalid"}}); err != nil {
		t.Fatal(err)
	}
	m.Close()
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := m.UIPermissions()
	if err != nil || state.Error == "" {
		t.Fatal("corruption ignored", err)
	}
	s := openUI(t, m, "example.native")
	s.Message([]byte(`{"kind":"ready","version":2}`))
	if _, err = s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("corrupt policy allowed access")
	}
	if _, err = m.SaveUIPermissions("example.native", state.Plugins[0].Fingerprint, state.Revision, []string{}); err == nil {
		t.Fatal("corrupt policy overwritten")
	}
	if m.settings.Get(uiPermissionsNamespace).Values["example.native"][0] != "invalid" {
		t.Fatal("original policy lost")
	}
}
func TestUIPermissionRevocationCancelsInFlightCommand(t *testing.T) {
	m := manager(t)
	s, _ := callableUI(t, m)
	callUI(t, s, `{"method":"command.execute","commandId":"echo","input":"warm"}`)
	done := make(chan error, 1)
	go func() {
		_, err := s.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"hang"}`))
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	state, _ := m.UIPermissions()
	plugin := state.Plugins[0]
	if _, err := m.SaveUIPermissions(plugin.ID, plugin.Fingerprint, state.Revision, []string{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("revoked command succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("revoked command did not terminate")
	}
}

func TestUIPermissionRevocationKeepsDataAndOtherPlugins(t *testing.T) {
	m := manager(t)
	session, _ := callableUI(t, m)
	callUI(t, session, `{"method":"storage.put","revision":0,"value":{"text":"keep"}}`)
	src := uiPackage(t, false, "0.1.0")
	mf, _ := manifest.Load(src)
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	mf.Contributes.UIViews[0].Permissions = []string{"storage"}
	writeManifest(t, src, mf)
	if _, err := m.Install(src, false); err != nil {
		t.Fatal(err)
	}
	grantAllUI(t, m, "example.hello")
	other := openUI(t, m, "example.hello")
	other.Message([]byte(`{"kind":"ready","version":2}`))
	state, _ := m.UIPermissions()
	var fingerprint string
	for _, plugin := range state.Plugins {
		if plugin.ID == "example.native" {
			fingerprint = plugin.Fingerprint
		}
	}
	if _, err := m.SaveUIPermissions("example.native", fingerprint, state.Revision, []string{}); err != nil {
		t.Fatal(err)
	}
	if session.Active() || !other.Active() {
		t.Fatal("revocation affected wrong sessions")
	}
	callUI(t, other, `{"method":"storage.get"}`)
	doc, err := m.documents.Get("example.native")
	if err != nil || doc.Revision != 1 {
		t.Fatal("revocation erased private data", err)
	}
}
