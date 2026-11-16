package catalog

import (
	"sinan/internal/buildinfo"
	"testing"
)

func TestPlatformCatalogHostVersion(t *testing.T) {
	original := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = original })
	release := Release{MinHostVersion: "0.3.0", Compatibility: Compatibility{API: "1", UI: "2"}}
	for _, tc := range []struct {
		version, reported string
		compatible        bool
	}{
		{"0.1.0", "0.1.0", false},
		{"0.2.0", "0.2.0", false},
		{"0.2.1-rc.1", "0.2.1-rc.1", false},
		{"0.2.1", "0.2.1", false},
		{"0.3.0-rc.1", "0.3.0-rc.1", false},
		{"0.3.0", "0.3.0", true},
		{"dev", "0.3.0", true},
		{"invalid", "0.0.0", false},
	} {
		buildinfo.Version = tc.version
		if HostVersion() != tc.reported || release.Compatible() != tc.compatible {
			t.Fatalf("incorrect host compatibility for %s", tc.version)
		}
	}
}
