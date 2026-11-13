package transport

import (
	"strings"
	"testing"
)

func TestPlatformEnvironmentAllowlist(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		// Unknown names deliberately have a nonempty synthetic value. A blanket
		// environment copy or a broad prefix match must not be needed here.
		queried := map[string]bool{}
		env := processEnvironment(platform, func(key string) string { queried[key] = true; return "fixture" })
		if queried["SINAN_TEST_PRIVATE_VALUE"] || queried["AWS_SECRET_ACCESS_KEY"] {
			t.Fatal("read variable outside allowlist")
		}
		found := map[string]bool{}
		for _, entry := range env {
			key, _, _ := strings.Cut(entry, "=")
			found[key] = true
		}
		if !found["LANG"] {
			t.Fatal("missing locale")
		}
		if platform == "windows" && (!found["APPDATA"] || !found["PROGRAMDATA"] || found["DISPLAY"]) {
			t.Fatal("wrong Windows environment")
		}
		if platform == "linux" && (!found["DISPLAY"] || !found["DBUS_SESSION_BUS_ADDRESS"] || found["APPDATA"]) {
			t.Fatal("wrong Linux environment")
		}
		if platform == "darwin" && len(env) != 3 {
			t.Fatal("unexpected macOS environment expansion")
		}
	}
	env := processEnvironment("darwin", func(string) string { return "" })
	if len(env) != 1 || env[0] != "LANG=en_US.UTF-8" {
		t.Fatal("missing fallback locale")
	}
}
