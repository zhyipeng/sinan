//go:build darwin && !ios && cgo

package uihost

import "testing"

func TestEmbeddedContentGeometryExcludesTrustedHeader(t *testing.T) {
	for _, test := range []struct {
		height            float64
		detached, flipped bool
		want              [4]float64
	}{
		{50, false, false, [4]float64{0, 0, 800, 0}},
		{488, false, false, [4]float64{0, 0, 800, 430}},
		{600, false, false, [4]float64{0, 0, 800, 542}},
		{700, false, false, [4]float64{0, 100, 800, 542}},
		{600, false, true, [4]float64{0, 58, 800, 542}},
		{800, true, false, [4]float64{0, 0, 800, 742}},
		{488, false, true, [4]float64{0, 58, 800, 430}},
	} {
		if got := embeddedContentRect(800, test.height, test.detached, test.flipped); got != test.want {
			t.Fatalf("body rect %v, want %v", got, test.want)
		}
	}
}
