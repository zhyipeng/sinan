package kernel

import (
	"context"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestClipboardAuthorityAndShape(t *testing.T) {
	permissions := []string{"clipboard.write"}
	calls := 0
	handler := clipboardHandler(permissions, func(ctx context.Context, method string, p protocol.ClipboardParams) (protocol.ClipboardResult, error) {
		calls++
		if method != "clipboard.write" || p.Content.Kind != "text" || string(p.Content.Data) != "fixture" {
			t.Fatal("wrong host call")
		}
		return protocol.ClipboardResult{}, nil
	})
	permissions[0] = "clipboard.read"
	for _, tc := range []struct{ method, raw string }{
		{"clipboard.read", `{}`},
		{"command.execute", `{}`},
		{"clipboard.write", `{"pluginId":"other"}`},
		{"clipboard.write", `{} {}`},
	} {
		if _, err := handler(context.Background(), tc.method, []byte(tc.raw)); err == nil {
			t.Fatal("invalid authority/shape accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid calls reached system")
	}
	if _, err := handler(context.Background(), "clipboard.write", []byte(`{"content":{"kind":"text","data":"Zml4dHVyZQ=="}}`)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("missing host call")
	}
}
func TestClipboardUIRequiresViewPermission(t *testing.T) {
	m := manager(t)
	session, _ := callableUI(t, m)
	for _, raw := range []string{`{"method":"clipboard.readImage"}`, `{"method":"clipboard.readText"}`, `{"method":"clipboard.writeText","input":"fixture"}`} {
		if _, err := session.Call(context.Background(), []byte(raw)); err == nil {
			t.Fatal("unprivileged UI reached clipboard")
		}
	}
}
