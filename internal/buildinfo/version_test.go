package buildinfo

import "testing"

func TestSemverPrecedence(t *testing.T) {
	ordered := []string{"0.9.0", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.10.0", "2.0.0", "1000000000000000000000000000.0.0"}
	for i, a := range ordered {
		if !ValidVersion(a) {
			t.Fatalf("invalid fixture %s", a)
		}
		for j, b := range ordered {
			got := Compare(a, b)
			if i == j && got != 0 || i < j && got >= 0 || i > j && got <= 0 {
				t.Fatalf("incorrect comparison %s / %s", a, b)
			}
		}
	}
	for _, value := range []string{"dev", "v1.0.0", "01.0.0", "1.0.0-rc.01", "1.0.0+meta", "1.0", "1.0.0/../x"} {
		if ValidVersion(value) {
			t.Fatalf("accepted %s", value)
		}
	}
}
