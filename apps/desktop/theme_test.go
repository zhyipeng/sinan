package main

import (
	"context"
	"sinan/internal/kernel"
	"sinan/internal/theme"
	"testing"
)

func TestThemeColourAndAppearanceAdmissionWithoutWindows(t *testing.T) {
	state := theme.Builtins("dark", 3)
	if themeColour(state, "dark") == themeColour(state, "light") {
		t.Fatal("palettes did not select different backgrounds")
	}
	manager, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	h := &HostService{manager: manager}
	// No application exists in this test; invalid and stale calls must stop before
	// the native/main-thread path. Normal headless preference saves remain usable.
	if err = h.ApplyThemeAppearance(context.Background(), 1, "dark"); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err = h.ApplyThemeAppearance(context.Background(), 0, "invalid"); err == nil {
		t.Fatal("invalid appearance accepted")
	}
	if _, err = h.SaveTheme("dark", 0); err != nil {
		t.Fatal(err)
	}
	if err = h.ApplyThemeAppearance(context.Background(), 1, "light"); err == nil {
		t.Fatal("manual theme contradicted by caller")
	}
	if err = h.ApplyThemeAppearance(context.Background(), 1, "dark"); err == nil {
		t.Fatal("missing caller window accepted")
	}
}
