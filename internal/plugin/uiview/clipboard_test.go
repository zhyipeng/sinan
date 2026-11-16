package uiview

import (
	"context"
	"encoding/json"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

func TestClipboardCallShapeBudgetAndRevocation(t *testing.T) {
	s := sessionFixture(t)
	s.binding.BridgeVersion = "2"
	calls := 0
	s.ConfigureCaller(func(ctx context.Context, p protocol.UIRequest) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`null`), nil
	})
	if _, err := s.Message([]byte(`{"kind":"ready","version":2}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"method":"clipboard.readText","pluginId":"other"}`,
		`{"method":"clipboard.readText","input":"extra"}`,
		`{"method":"clipboard.writeText","commandId":"other","input":"fixture"}`,
		`{"method":"clipboard.writeText","input":null}`,
		`{"method":"clipboard.writeText","input":"` + strings.Repeat("a", (32<<10)+1) + `"}`,
	} {
		if _, err := s.Call(context.Background(), []byte(raw)); err == nil {
			t.Fatal("invalid request admitted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid call reached host")
	}
	for _, raw := range []string{`{"method":"clipboard.readText"}`, `{"method":"clipboard.writeText","input":"fixture"}`} {
		if _, err := s.Call(context.Background(), []byte(raw)); err != nil {
			t.Fatal(err)
		}
	}
	s.Revoke()
	if _, err := s.Call(context.Background(), []byte(`{"method":"clipboard.readText"}`)); err == nil {
		t.Fatal("revoked session retained authority")
	}
	if calls != 2 {
		t.Fatal("wrong call count")
	}
}
