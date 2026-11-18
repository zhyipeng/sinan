package main

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/kernel"
)

// Fixtures are the reported three-display layout, converted from NSScreen
// visibleFrame to the same logical coordinate space used by SetPosition.
func TestLauncherPositionOnReportedDisplays(t *testing.T) {
	for _, test := range []struct {
		name string
		area application.Rect
		x, y int
	}{
		{"Built-in Retina", application.Rect{X: 0, Y: 33, Width: 1512, Height: 949}, 356, 222},
		{"DELL P2723D portrait", application.Rect{X: 1512, Y: -1440, Width: 1440, Height: 2560}, 1832, -928},
		{"DELL U2719D above-left", application.Rect{X: -1048, Y: -1440, Width: 2560, Height: 1440}, -168, -1152},
	} {
		t.Run(test.name, func(t *testing.T) {
			x, y := launcherPosition(test.area, kernel.DefaultLauncherTopOffset)
			if x != test.x || y != test.y {
				t.Fatalf("position = (%d, %d), want (%d, %d)", x, y, test.x, test.y)
			}
		})
	}
}

func TestDesktopLauncherPositionUsesConfiguredOffset(t *testing.T) {
	area := application.Rect{X: -1048, Y: -1400, Width: 2560, Height: 1401}
	for percent, wantY := range map[int]int{0: -1400, 13: -1218, 50: -700} {
		x, y := launcherPosition(area, percent)
		if x != -168 || y != wantY {
			t.Errorf("offset %d%%: got (%d, %d), want (-168, %d)", percent, x, y, wantY)
		}
	}
}

func TestDesktopLauncherFitsExpandedViewWithinWorkArea(t *testing.T) {
	for _, test := range []struct {
		name       string
		area       application.Rect
		wantY      int
		wantHeight int
	}{
		{"above primary display", application.Rect{Y: -900, Height: 800}, -716, 600},
		{"display shorter than settings", application.Rect{Y: 32, Height: 520}, 32, 504},
		{"room to grow downward", application.Rect{Y: 22, Height: 1440}, 742, 600},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, preferredY := launcherPosition(test.area, 50)
			y, height := fitLauncherVertically(test.area, preferredY, launcherMaxHeight)
			if y != test.wantY || height != test.wantHeight {
				t.Fatalf("expanded view = (y: %d, height: %d), want (%d, %d)", y, height, test.wantY, test.wantHeight)
			}
			if y, height = fitLauncherVertically(test.area, preferredY, launcherMinHeight); y != preferredY || height != launcherMinHeight {
				t.Fatalf("collapsed view = (y: %d, height: %d), want (%d, %d)", y, height, preferredY, launcherMinHeight)
			}
		})
	}
}

func TestDirectoryDialogDefersMainPanelBlurOnly(t *testing.T) {
	main := &application.WebviewWindow{}
	other := &application.WebviewWindow{}
	l := &launcher{window: main}
	if !l.shouldHideOnBlur(main) || l.shouldHideOnBlur(other) {
		t.Fatal("unexpected ordinary blur policy")
	}
	finish := l.beginDialog(main)
	nested := l.beginDialog(main)
	if l.shouldHideOnBlur(main) {
		t.Fatal("dialog dismissed its owner")
	}
	finish()
	if l.shouldHideOnBlur(main) {
		t.Fatal("nested dialog lost focus protection")
	}
	nested()
	if !l.shouldHideOnBlur(main) {
		t.Fatal("blur policy was not restored")
	}
	finishOther := l.beginDialog(other)
	if !l.shouldHideOnBlur(main) {
		t.Fatal("detached dialog changed main panel policy")
	}
	finishOther()
}

func TestDesktopLauncherIgnoresStaleStartupPresentation(t *testing.T) {
	initial := &application.WebviewWindow{}
	replacement := &application.WebviewWindow{}
	l := &launcher{window: replacement}

	l.showAtStartup(initial)
	if l.retention.epoch != 0 {
		t.Fatal("startup callback changed the replacement window's visibility session")
	}

	// Simulate a shortcut opening and dismissing the panel before runtime ready.
	l.window = initial
	l.retention.shown()
	l.retention.hidden(time.Now())
	epoch := l.retention.epoch
	l.showAtStartup(initial)
	if l.retention.epoch != epoch || l.retention.hiddenAt.IsZero() {
		t.Fatal("startup callback reopened the dismissed search panel")
	}
}
