package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Fixtures are the reported three-display layout, converted from NSScreen
// visibleFrame to the same logical coordinate space used by SetPosition.
func TestLauncherPositionOnReportedDisplays(t *testing.T) {
	for _, test := range []struct {
		name string
		area application.Rect
		x, y int
	}{
		{"Built-in Retina", application.Rect{X: 0, Y: 33, Width: 1512, Height: 949}, 416, 222},
		{"DELL P2723D portrait", application.Rect{X: 1512, Y: -1440, Width: 1440, Height: 2560}, 1892, -928},
		{"DELL U2719D above-left", application.Rect{X: -1048, Y: -1440, Width: 2560, Height: 1440}, -108, -1152},
	} {
		t.Run(test.name, func(t *testing.T) {
			x, y := launcherPosition(test.area)
			if x != test.x || y != test.y {
				t.Fatalf("position = (%d, %d), want (%d, %d)", x, y, test.x, test.y)
			}
		})
	}
}
