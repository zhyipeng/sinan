package hostupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func cacheForTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("XDG_CACHE_HOME", root)
}
func fakePackage(t *testing.T, target, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(metadataPath(target)), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(Metadata{Version: version, Platform: Platform(), Arch: Arch()})
	if err := os.WriteFile(metadataPath(target), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(BinaryDir(target), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sinan", "sinanctl", "sinan-ui-host", "sinan-updater"} {
		if err := os.WriteFile(filepath.Join(BinaryDir(target), name+suffix()), []byte(version), 0700); err != nil {
			t.Fatal(err)
		}
	}
}
func planForTest(t *testing.T) Plan {
	t.Helper()
	cacheForTest(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := CacheRoot()
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	task, err := os.MkdirTemp(root, "install-")
	if err != nil {
		t.Fatal(err)
	}
	task, err = filepath.EvalSymlinks(task)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{Target: filepath.Join(base, ArchiveRoot()), Work: filepath.Join(base, ".sinan-update-123456"), Task: task, Version: "0.2.0", Previous: "0.1.0", Parent: os.Getpid()}
	fakePackage(t, p.Target, p.Previous)
	fakePackage(t, p.Stage(), p.Version)
	return p
}
func TestReplacementKeepsBackupAndRollsBackUnhealthyVersion(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "rollback"}[healthy], func(t *testing.T) {
			p := planForTest(t)
			launches := 0
			err := ReplaceAndRestart(p, func(executable, session string) bool {
				launches++
				if !strings.HasPrefix(executable, p.Target+string(os.PathSeparator)) {
					t.Fatal("launch outside installation")
				}
				if session == "" {
					return true
				}
				return healthy
			})
			meta, readErr := ReadMetadata(p.Target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if healthy {
				if err != nil || meta.Version != p.Version || launches != 1 {
					t.Fatalf("success: %v / %s / %d", err, meta.Version, launches)
				}
				if _, err := os.Stat(p.Backup()); err != nil {
					t.Fatal("old directory not retained")
				}
			} else {
				if err == nil || meta.Version != p.Previous || launches != 2 {
					t.Fatalf("rollback: %v / %s / %d", err, meta.Version, launches)
				}
			}
		})
	}
}
func TestPlanRejectsPlatformDowngradeAndUnsafePaths(t *testing.T) {
	p := planForTest(t)
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	original := p.Version
	p.Version = p.Previous
	if p.validate() == nil {
		t.Fatal("accepted downgrade")
	}
	p.Version = original
	p.Work = filepath.Dir(p.Target)
	if p.validate() == nil {
		t.Fatal("accepted unbounded replacement directory")
	}
}
func TestArchiveRejectsTraversalLinksAndCollisions(t *testing.T) {
	for _, entry := range []string{"../outside", ArchiveRoot() + "/../outside", ArchiveRoot() + "/file:stream", ArchiveRoot() + "/NUL", ArchiveRoot() + "/data.", "other/file"} {
		t.Run(strings.ReplaceAll(entry, "/", "_"), func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "test.tar.gz")
			file, _ := os.Create(archive)
			gz := gzip.NewWriter(file)
			tw := tar.NewWriter(gz)
			_ = tw.WriteHeader(&tar.Header{Name: entry, Mode: 0600, Size: 1})
			_, _ = tw.Write([]byte("x"))
			_ = tw.Close()
			_ = gz.Close()
			_ = file.Close()
			if Extract(archive, t.TempDir(), false) == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
	archive := filepath.Join(t.TempDir(), "links.tar.gz")
	file, _ := os.Create(archive)
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: ArchiveRoot() + "/link", Typeflag: tar.TypeSymlink, Linkname: "/outside"})
	_ = tw.Close()
	_ = gz.Close()
	_ = file.Close()
	if Extract(archive, t.TempDir(), false) == nil {
		t.Fatal("accepted symlink")
	}
	for _, names := range [][]string{{"Data", "data"}, {"a/b", "a"}, {"A/b", "a/c"}} {
		archive := filepath.Join(t.TempDir(), "collision.zip")
		file, _ := os.Create(archive)
		zw := zip.NewWriter(file)
		for _, name := range names {
			writer, _ := zw.Create(ArchiveRoot() + "/" + name)
			_, _ = writer.Write([]byte("x"))
		}
		_ = zw.Close()
		_ = file.Close()
		if Extract(archive, t.TempDir(), true) == nil {
			t.Fatal("accepted path collision")
		}
	}
}
func TestHealthyReceiptRequiresMatchingVersionAndInstallation(t *testing.T) {
	p := planForTest(t)
	if err := writeJSON(filepath.Join(p.Task, "plan.json"), p); err != nil {
		t.Fatal(err)
	}
	if ConfirmStartup(filepath.Base(p.Task), p.Previous, filepath.Join(BinaryDir(p.Target), "sinan"+suffix())) == nil {
		t.Fatal("old host acknowledged new version")
	}
	// No desktop process is launched; only the receipt and identity checks run.
	if err := os.Rename(p.Target, p.Backup()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(p.Stage(), p.Target); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmStartup(filepath.Base(p.Task), p.Version, filepath.Join(BinaryDir(p.Target), "sinan"+suffix())); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(p.Task, "healthy"))
	if err != nil || string(data) != p.Version {
		t.Fatal("missing health receipt")
	}
	if runtime.GOOS == "darwin" && filepath.Ext(p.Target) != ".app" {
		t.Fatal("fixture is not a bundle")
	}
}

// Opt-in integration validates an actual signed archive without starting a host.
func TestPackagedArchiveWhenProvided(t *testing.T) {
	archive := os.Getenv("SINAN_UPDATE_TEST_ARCHIVE")
	if archive == "" {
		t.Skip("未指定真实发行归档；不启动 UI")
	}
	p := planForTest(t)
	plan, err := StageArchive(archive, p.Target, p.Version, p.Previous)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Cleanup()
	if err = ValidatePackage(plan.Stage(), p.Version); err != nil {
		t.Fatal(err)
	}
}
