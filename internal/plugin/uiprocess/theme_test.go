package uiprocess

import (
	"context"
	"encoding/json"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestThemeUpdatesPreserveGuestAndRejectStaleState(t *testing.T) {
	s := session(t)
	initial := theme.Builtins("light", 3)
	config := options("wait-theme")
	config.Theme = &initial
	p, err := Start(context.Background(), s, config)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	next := theme.Builtins("dark", 5)
	if err = p.SetTheme(next); err != nil {
		t.Fatal(err)
	}
	if err = p.SetTheme(theme.Builtins("system", 4)); err != nil {
		t.Fatal(err)
	}
	invalid := theme.CloneState(next)
	invalid.Dark.Tokens["background"] = "url(invalid)"
	if err = p.SetTheme(invalid); err == nil {
		t.Fatal("unsafe presentation accepted")
	}
	// Focus ack follows the theme frames in the private pipe, establishing receipt.
	if err = p.Focus(); err != nil {
		t.Fatal("guest did not remain interactive", err)
	}
	if !s.Active() {
		t.Fatal("theme change revoked the document")
	}
	s.Revoke()
	result := p.Wait()
	var received protocol.ThemeState
	if result.Err != nil || json.Unmarshal(result.Report, &received) != nil || received.Mode != "dark" || received.Revision != 5 {
		t.Fatal("guest received stale state", result.Err)
	}
}
