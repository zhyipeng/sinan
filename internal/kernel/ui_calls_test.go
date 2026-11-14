package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func callableUI(t *testing.T, m *Manager) (*uiview.Session, string) {
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
	s := openUI(t, m, "example.native")
	if _, err := s.Message([]byte(`{"kind":"ready","version":2}`)); err != nil {
		t.Fatal(err)
	}
	return s, src
}
func callUI(t *testing.T, s *uiview.Session, request string) json.RawMessage {
	t.Helper()
	result, err := s.Call(context.Background(), []byte(request))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestUICommandsAndDataAreSessionBound(t *testing.T) {
	m := manager(t)
	s, src := callableUI(t, m)
	result := callUI(t, s, `{"method":"command.execute","commandId":"echo","input":"text"}`)
	if !strings.Contains(string(result), "text") {
		t.Fatal("wrong command result")
	}
	callUI(t, s, `{"method":"storage.put","revision":0,"value":{"draft":"persisted"}}`)
	for _, request := range []string{
		`{"method":"command.execute","commandId":"crash"}`,
		`{"method":"command.execute","commandId":"echo","pluginId":"system.settings"}`,
		`{"method":"storage.get","path":"../settings.json"}`,
		`{"method":"storage.put","revision":0,"value":null}`,
		`{"method":"storage.get","method":"storage.get"}`,
		`{"method":"storage.put","revision":1,"value":null,"input":"ignored"}`,
		`{"method":"Install"}`,
	} {
		if _, err := s.Call(context.Background(), []byte(request)); err == nil {
			t.Fatal("unauthorized or malformed call accepted")
		}
	}
	if _, err := m.Reload("example.native"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("old session revived")
	}
	next := openUI(t, m, "example.native")
	next.Message([]byte(`{"kind":"ready","version":2}`))
	if !strings.Contains(string(callUI(t, next, `{"method":"storage.get"}`)), "persisted") {
		t.Fatal("reload lost data")
	}
	// Updating the package keeps private data, but old session objects stay revoked.
	mf, _ := manifest.Load(src)
	mf.Version = "0.2.0"
	writeManifest(t, src, mf)
	if _, err := m.Upgrade(src, true); err != nil {
		t.Fatal(err)
	}
	next = openUI(t, m, "example.native")
	next.Message([]byte(`{"kind":"ready","version":2}`))
	if !strings.Contains(string(callUI(t, next, `{"method":"storage.get"}`)), "persisted") {
		t.Fatal("upgrade lost data")
	}
	if err := os.WriteFile(filepath.Join(m.root, "example.native", "changed"), []byte("change"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil || next.Active() {
		t.Fatal("changed package retained authority")
	}
}
func TestUICommandTimeoutAndRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "revoke"}[revoke], func(t *testing.T) {
			m := manager(t)
			s, _ := callableUI(t, m)
			// Warm the process before timing the bounded call.
			callUI(t, s, `{"method":"command.execute","commandId":"echo","input":"ready"}`)
			done := make(chan error, 1)
			go func() {
				_, err := s.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"hang"}`))
				done <- err
			}()
			if revoke {
				time.Sleep(30 * time.Millisecond)
				s.Revoke()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("failed command succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("UI command did not terminate")
			}
		})
	}
}
func TestUIRequiresReadyAndDeclaredCapabilities(t *testing.T) {
	m := manager(t)
	src := uiPackage(t, false, "0.1.0")
	mf, _ := manifest.Load(src)
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	writeManifest(t, src, mf)
	m.Install(src, false)
	s := openUI(t, m, "example.hello")
	if _, err := s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("call before ready")
	}
	s.Message([]byte(`{"kind":"ready","version":2}`))
	if _, err := s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("undeclared storage accepted")
	}
	if _, err := s.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"hello"}`)); err == nil {
		t.Fatal("undeclared command accepted")
	}
}

func TestUIAdmissionDoesNotWaitForLifecycleLocks(t *testing.T) {
	m := manager(t)
	s, _ := callableUI(t, m)
	for _, lockPlugin := range []bool{false, true} {
		done := make(chan error, 1)
		p, _ := m.lookup("example.native")
		if lockPlugin {
			p.mu.Lock()
		} else {
			m.mutationMu.Lock()
		}
		go func() { _, err := s.Call(context.Background(), []byte(`{"method":"storage.get"}`)); done <- err }()
		select {
		case err := <-done:
			if lockPlugin {
				p.mu.Unlock()
			} else {
				m.mutationMu.Unlock()
			}
			if err == nil {
				t.Fatal("busy lifecycle accepted")
			}
		case <-time.After(250 * time.Millisecond):
			if lockPlugin {
				p.mu.Unlock()
			} else {
				m.mutationMu.Unlock()
			}
			<-done
			t.Fatal("UI request waited for lifecycle lock")
		}
	}
}

func TestPasteRequiresDeclarationAndLiveSession(t *testing.T) {
	m := manager(t)
	s, _ := callableUI(t, m)
	calls := 0
	m.SetUIHostCall(func(_ context.Context, _ *uiview.Session, _ protocol.UIRequest) (json.RawMessage, error) {
		calls++
		return json.RawMessage("null"), nil
	})
	if _, err := s.Call(context.Background(), []byte(`{"method":"input.paste"}`)); err == nil || calls != 0 {
		t.Fatal("undeclared input accepted")
	}
	src := uiPackage(t, false, "0.1.0")
	mf, _ := manifest.Load(src)
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	mf.Contributes.UIViews[0].Permissions = []string{"input.paste"}
	writeManifest(t, src, mf)
	if _, err := m.Install(src, false); err != nil {
		t.Fatal(err)
	}
	s = openUI(t, m, "example.hello")
	s.Message([]byte(`{"kind":"ready","version":2}`))
	callUI(t, s, `{"method":"input.paste"}`)
	if calls != 1 {
		t.Fatal("declared input not routed")
	}
	for _, request := range []string{`{"method":"input.paste","input":"other"}`, `{"method":"input.paste","value":null}`, `{"method":"input.paste","commandId":"open"}`} {
		if _, err := s.Call(context.Background(), []byte(request)); err == nil {
			t.Fatal("paste accepted payload")
		}
	}
	s.Revoke()
	if _, err := s.Call(context.Background(), []byte(`{"method":"input.paste"}`)); err == nil || calls != 1 {
		t.Fatal("revoked input accepted")
	}
}
