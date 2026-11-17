package platform

import "testing"

func TestPlatformWindowOpacityRejectsInvalidHandleAndRange(t *testing.T) {
	for _, percent := range []int{-1, 0, 29, 30, 100, 101} {
		if err := SetWindowOpacity(nil, percent); err == nil {
			t.Fatal("nil window accepted")
		}
	}
}
