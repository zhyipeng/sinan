package uiview

import (
	"context"
	"encoding/json"
	protocol "sinan/packages/plugin-protocol"
	"testing"
	"time"
)

func windowSession(t *testing.T) *Session {
	t.Helper()
	s := sessionFixture(t)
	s.binding.BridgeVersion = "2"
	if _, err := s.Message([]byte(`{"kind":"ready","version":2}`)); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestWindowExecutionBudgetIsScopedAndRevocable(t *testing.T) {
	s := windowSession(t)
	s.ConfigureCaller(func(ctx context.Context, req protocol.UIRequest) (json.RawMessage, error) {
		deadline, _ := ctx.Deadline()
		remaining := time.Until(deadline)
		if req.Method == "window.execute" {
			if remaining < 120*time.Second || remaining > 125*time.Second {
				t.Error("wrong window deadline", remaining)
			}
			select {
			case <-time.After(600 * time.Millisecond):
				return json.RawMessage(`null`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if remaining > 500*time.Millisecond {
			t.Error("ordinary call deadline expanded", remaining)
		}
		return json.RawMessage(`null`), nil
	})
	if _, err := s.Call(context.Background(), []byte(`{"method":"window.execute","commandId":"region"}`)); err != nil {
		t.Fatal("long window operation failed", err)
	}
	if _, err := s.Call(context.Background(), []byte(`{"method":"command.execute","commandId":"region"}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"method":"window.execute"}`, `{"method":"window.execute","commandId":"region","revision":0}`, `{"method":"window.execute","commandId":"region","input":null}`, `{"method":"window.execute","commandId":"region","windowId":"other"}`} {
		if _, err := s.Call(context.Background(), []byte(raw)); err == nil {
			t.Fatal("malformed request accepted")
		}
	}
	entered, done := make(chan struct{}), make(chan error, 1)
	s.ConfigureCaller(func(ctx context.Context, _ protocol.UIRequest) (json.RawMessage, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	go func() {
		_, err := s.Call(context.Background(), []byte(`{"method":"window.execute","commandId":"region"}`))
		done <- err
	}()
	<-entered
	s.Revoke()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("revoked task succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not cancel long task")
	}
}

func TestImageOutputBridgeRejectsExtraFields(t *testing.T) {
	s := windowSession(t)
	calls := 0
	s.ConfigureCaller(func(ctx context.Context, req protocol.UIRequest) (json.RawMessage, error) {
		calls++
		deadline, _ := ctx.Deadline()
		if (req.Method == "file.savePNG" || req.Method == "file.openImage" || req.Method == "file.pickPath") && time.Until(deadline) < 120*time.Second {
			t.Fatal("native dialog has ordinary short timeout")
		}
		return json.RawMessage(`null`), nil
	})
	for _, raw := range []string{`{"method":"window.show","input":"other"}`, `{"method":"window.hide","windowId":"other"}`, `{"method":"file.openImage","input":"/arbitrary/path"}`, `{"method":"file.pickPath"}`, `{"method":"file.pickPath","input":"/arbitrary/path"}`, `{"method":"file.pickPath","input":"directory","value":{}}`, `{"method":"file.savePNG"}`, `{"method":"file.savePNG","commandId":"save","value":null}`, `{"method":"file.savePNG","commandId":"save","path":"/tmp/other.png"}`} {
		if _, err := s.Call(context.Background(), []byte(raw)); err == nil {
			t.Fatal("invalid request accepted", raw)
		}
	}
	if calls != 0 {
		t.Fatal("malformed request reached host")
	}
	for _, raw := range []string{`{"method":"window.show"}`, `{"method":"window.hide"}`, `{"method":"file.openImage"}`, `{"method":"file.pickPath","input":"file"}`, `{"method":"file.pickPath","input":"directory"}`, `{"method":"file.savePNG","commandId":"save","input":"opaque"}`} {
		if _, err := s.Call(context.Background(), []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDependencyBridgeStrictFields(t *testing.T) {
	s := windowSession(t)
	calls := 0
	s.ConfigureCaller(func(context.Context, protocol.UIRequest) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`null`), nil
	})
	for _, raw := range []string{`{"method":"dependency.status","input":"other"}`, `{"method":"dependency.open","input":"ocr","value":{},"pluginId":"other.plugin"}`, `{"method":"image.upload.start","input":"ocr"}`, `{"method":"image.upload.chunk","input":"invalid"}`, `{"method":"image.upload.finish","input":"short"}`, `{"method":"dependency.manage","value":null}`} {
		if _, err := s.Call(context.Background(), []byte(raw)); err == nil {
			t.Fatal("malformed dependency request accepted", raw)
		}
	}
	if calls != 0 {
		t.Fatal("malformed request reached host")
	}
}
