package uiwindows

import (
	"encoding/json"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

func TestControllerThemeBeforeAndAfterStartup(t *testing.T) {
	m := installed(t, "")
	c, _ := controller(t, m, "wait-theme")
	c.SetTheme(theme.Builtins("dark", 4))
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	entry := c.panel
	c.mu.Unlock()
	c.SetTheme(theme.Builtins("light", 6))
	c.SetTheme(theme.Builtins("system", 5))
	if err := entry.process.Focus(); err != nil {
		t.Fatal(err)
	}
	if !entry.session.Active() {
		t.Fatal("theme update replaced the session")
	}
	c.mu.Lock()
	still := c.panel == entry
	c.mu.Unlock()
	if !still {
		t.Fatal("theme update replaced page")
	}
	entry.session.Revoke()
	result := entry.process.Wait()
	var received protocol.ThemeState
	if result.Err != nil || json.Unmarshal(result.Report, &received) != nil || received.Mode != "light" || received.Revision != 6 {
		t.Fatal("incorrect theme", result.Err)
	}
}
