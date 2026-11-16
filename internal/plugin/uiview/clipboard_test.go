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
		`{"method":"clipboard.readImage","input":"extra"}`,
		`{"method":"command.executeImage","commandId":"start","value":{}}`,
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

func TestImageSnapshotReplacementAndRevoke(t *testing.T) {
	s := sessionFixture(t)
	first := s.AttachLaunchImage([]byte("first"), 1, 1)
	data, ok := s.Image(first.URL)
	if !ok || string(data) != "first" {
		t.Fatal("missing snapshot")
	}
	data[0] = 'x'
	same, _ := s.Image(first.URL)
	if string(same) != "first" {
		t.Fatal("mutable snapshot")
	}
	second := s.AttachLaunchImage([]byte("second"), 2, 2)
	if first.URL == second.URL {
		t.Fatal("reused image URL")
	}
	if _, ok = s.Image(first.URL); ok {
		t.Fatal("old image still accessible")
	}
	if _, ok = s.Resource(first.URL); ok {
		t.Fatal("old resource still accessible")
	}
	if _, ok = s.Image(s.EntryURL()); ok {
		t.Fatal("bundle resource accepted as image")
	}
	s.Revoke()
	if _, ok = s.Image(second.URL); ok {
		t.Fatal("revoked image accessible")
	}
}
