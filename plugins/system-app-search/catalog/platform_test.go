package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	protocol "sinan/packages/plugin-protocol"
)

func TestPlatformRoots(t *testing.T) {
	home := t.TempDir()
	data := filepath.Join(home, "data")
	common := filepath.Join(home, "common")
	getenv := func(key string) string {
		return map[string]string{"APPDATA": data, "PROGRAMDATA": common, "XDG_DATA_HOME": data, "XDG_DATA_DIRS": common + ":relative:" + data}[key]
	}
	homeDir := func() (string, error) { return home, nil }
	windows := defaultRoots("windows", getenv, homeDir)
	if len(windows) != 2 || windows[0] != filepath.Join(data, "Microsoft", "Windows", "Start Menu", "Programs") {
		t.Fatal(windows)
	}
	// XDG uses POSIX absolute paths and colon separators; Windows drive letters
	// intentionally cannot stand in for a native Linux filesystem in this test.
	if runtime.GOOS == "windows" {
		return
	}
	linux := defaultRoots("linux", getenv, homeDir)
	if !reflect.DeepEqual(linux, []string{filepath.Join(data, "applications"), filepath.Join(common, "applications"), filepath.Join(data, "applications")}) {
		t.Fatal(linux)
	}
	linux = defaultRoots("linux", func(string) string { return "" }, homeDir)
	if len(linux) != 3 || linux[0] != filepath.Join(home, ".local", "share", "applications") {
		t.Fatal(linux)
	}
}

func TestDesktopMetadata(t *testing.T) {
	base := "[Desktop Entry]\nType=Application\nName=Editor\nName[zh_CN]=编辑器\nExec=editor %F\n"
	lookup := func(string) (string, error) { return "", errors.New("missing") }
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"localized", base, true},
		{"hidden", base + "Hidden=true\n", false},
		{"no-display", base + "NoDisplay=true\n", false},
		{"missing-executable", base + "TryExec=missing\n", false},
		{"other-desktop", base + "OnlyShowIn=KDE;\n", false},
		{"current-desktop", base + "OnlyShowIn=GNOME;\n", true},
		{"excluded-desktop", base + "NotShowIn=GNOME;\n", false},
		{"wrong-type", strings.Replace(base, "Type=Application", "Type=Link", 1), false},
		{"duplicate", base + "Name=Duplicate\n", false},
		{"missing-exec", strings.Replace(base, "Exec=editor %F", "", 1), false},
		{"dbus", strings.Replace(base, "Exec=editor %F", "DBusActivatable=true", 1), true},
		{"oversized", base + strings.Repeat("x", maxDesktopEntryBytes), false},
		{"actions-ignored", base + "[Desktop Action Hidden]\nName=Other\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := desktopMetadata(strings.NewReader(tc.content), "zh_CN.UTF-8", "ubuntu:GNOME", lookup)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && meta.DisplayName != "编辑器" {
				t.Fatal(meta)
			}
		})
	}
	if got := desktopLocales("sr_RS.UTF-8@latin"); !reflect.DeepEqual(got, []string{"sr_RS@latin", "sr_RS", "sr@latin", "sr"}) {
		t.Fatal(got)
	}
}

func TestPlatformFileDiscoveryCacheAndLaunch(t *testing.T) {
	for _, target := range []struct{ platform, extension string }{{"windows", ".lnk"}, {"windows", ".exe"}, {"linux", ".desktop"}} {
		t.Run(target.platform+target.extension, func(t *testing.T) {
			root, cache := t.TempDir(), t.TempDir()
			path := filepath.Join(root, "Editor"+target.extension)
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			// A directory with an application-file suffix must never become launchable.
			if err := os.Mkdir(filepath.Join(root, "Wrong"+target.extension), 0700); err != nil {
				t.Fatal(err)
			}
			c := NewWithCache([]string{root}, cache)
			c.platform = target.platform
			c.read = func(string) (metadata, error) { return metadata{Name: "Editor"}, nil }
			p := protocol.SearchParams{ProviderID: "applications", Query: "editor", Limit: 20}
			result, err := c.Search(context.Background(), p)
			if err != nil || len(result.Items) != 1 {
				t.Fatalf("discovery: %v %+v", err, result)
			}
			launched := ""
			c.launch = func(_ context.Context, path string) error { launched = path; return nil }
			if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: path}); err == nil {
				t.Fatal("accepted raw path")
			}
			if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: result.Items[0].ID}); err != nil || launched != path {
				t.Fatal("indexed file launch failed", err)
			}
			restored := NewWithCache([]string{root}, cache)
			if !restored.indexed || len(restored.apps) != 1 || restored.apps[0].ID != result.Items[0].ID {
				t.Fatal("file snapshot not restored")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			launched = ""
			if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: result.Items[0].ID}); err == nil || launched != "" {
				t.Fatal("removed file launched")
			}
		})
	}
}

func TestDesktopUserEntryMasksSystemEntry(t *testing.T) {
	userRoot, systemRoot := t.TempDir(), t.TempDir()
	for _, root := range []string{userRoot, systemRoot} {
		if err := os.WriteFile(filepath.Join(root, "editor.desktop"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := New([]string{userRoot, systemRoot})
	c.platform = "linux"
	c.read = func(path string) (metadata, error) {
		if filepath.Dir(path) == userRoot {
			return metadata{}, errors.New("hidden user entry")
		}
		return metadata{Name: "Editor"}, nil
	}
	result, err := c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "editor", Limit: 20})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("hidden entry did not mask system app: %v", err)
	}
}
