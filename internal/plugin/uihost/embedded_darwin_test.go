//go:build darwin && !ios && cgo

package uihost

import "testing"

func TestEmbeddedContentGeometryExcludesTrustedHeader(t *testing.T) {
	for _, test := range []struct {
		height            float64
		detached, flipped bool
		want              [4]float64
	}{
		{50, false, false, [4]float64{0, 0, 680, 0}},
		{488, false, false, [4]float64{0, 0, 680, 430}},
		{560, false, false, [4]float64{0, 72, 680, 430}},
		{800, true, false, [4]float64{0, 0, 680, 742}},
		{488, false, true, [4]float64{0, 58, 680, 430}},
	} {
		if got := embeddedContentRect(680, test.height, test.detached, test.flipped); got != test.want {
			t.Fatalf("body rect %v, want %v", got, test.want)
		}
	}
}
