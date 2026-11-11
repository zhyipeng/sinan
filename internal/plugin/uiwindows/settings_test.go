package uiwindows

import (
	"testing"
	"time"
)

func TestSettingsNavigationReleasesOnlyAttachedPanel(t *testing.T) {
	for _, attached := range []bool{true, false} {
		name := "detached"
		if attached {
			name = "attached"
		}
		t.Run(name, func(t *testing.T) {
			m := installed(t, "")
			c, events := controller(t, m, "settings")
			c.options.Panel = attached
			if err := c.Open("example.ui", "main"); err != nil {
				t.Fatal(err)
			}
			event := waitEvent(t, events, 2*time.Second)
			if !event.OpenSettings || event.ReturnToSearch {
				t.Fatal("settings request did not stay separate from close", event)
			}
			c.mu.Lock()
			entry := c.opening["example.ui/main"]
			c.mu.Unlock()
			if entry == nil {
				t.Fatal("settings request closed the guest")
			}
			c.ReleaseMain()
			if attached && entry.session.Active() {
				t.Fatal("attached panel retained the main slot")
			}
			if !attached && !entry.session.Active() {
				t.Fatal("detached document was revoked")
			}
			if handled, _ := c.Main(true); handled {
				t.Fatal("settings navigation retained an attached panel")
			}
			if !attached {
				if err := entry.process.Focus(); err != nil {
					t.Fatal("independent guest stopped responding", err)
				}
			}
		})
	}
}
