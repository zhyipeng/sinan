package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

func packageVersion(t *testing.T, native bool, version string) string {
	t.Helper()
	dir := source(t, native)
	mf, _ := manifest.Load(dir)
	mf.Version = version
	if native {
		mf.Contributes.Settings = []manifest.Setting{{ID: "directories", Title: "Directories", Type: "directory-list"}}
	}
	writeManifest(t, dir, mf)
	return dir
}
func writeManifest(t *testing.T, dir string, mf manifest.Manifest) {
	t.Helper()
	data, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestUpgradeKeepsSettingsHistoryAndRetiresSearch(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(packageVersion(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := m.SavePluginSettings("example.native", 0, map[string][]string{"directories": {directory}}); err != nil {
		t.Fatal(err)
	}
	m.Execute(context.Background(), "example.native", "echo", "usage")
	before, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil {
		t.Fatal(err)
	}
	candidate := packageVersion(t, true, "0.2.0")
	preview, err := m.PreviewPackage(candidate)
	if err != nil || !preview.Changed || preview.InstalledVersion != "0.1.0" {
		t.Fatal("bad preview", err)
	}
	if _, err = m.Upgrade(candidate, false); err == nil {
		t.Fatal("upgrade bypassed native approval")
	}
	pending := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		pending <- err
	}()
	awaitSearch(t, m, "example.native")
	info, err := m.ApplyPackage(candidate, true, preview.Fingerprint, preview.InstalledFingerprint)
	if err != nil || info.Version != "0.2.0" || info.Changed {
		t.Fatal("upgrade failed", err)
	}
	select {
	case err := <-pending:
		if err == nil {
			t.Fatal("old query survived upgrade")
		}
	case <-time.After(time.Second):
		t.Fatal("old query hung")
	}
	after, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil || after.Text == before.Text {
		t.Fatal("old process still used", err)
	}
	config, _ := m.PluginSettings("example.native")
	if config.Revision != 1 || config.Values["directories"][0] != directory || m.UsageBoost("example.native", "echo", "usage") == 0 {
		t.Fatal("host data lost")
	}
	if !m.nativeApproved("example.native") {
		t.Fatal("native approval lost")
	}
}

func TestPluginCacheDirectorySurvivesUpgradeAndFailedCacheCreation(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(packageVersion(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	cache, err := m.Execute(context.Background(), "example.native", "echo", "cacheDir")
	expected := filepath.Join(m.root, ".sinan-state", "cache", "example.native")
	if err != nil || cache.Text != expected {
		t.Fatal("cache directory not passed during initialize", err)
	}
	marker := filepath.Join(expected, "fixture")
	if err := os.WriteFile(marker, []byte("cached"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Upgrade(packageVersion(t, true, "0.2.0"), true); err != nil {
		t.Fatal(err)
	}
	after, err := m.Execute(context.Background(), "example.native", "echo", "cacheDir")
	data, readErr := os.ReadFile(marker)
	if err != nil || after.Text != expected || readErr != nil || string(data) != "cached" {
		t.Fatal("upgrade lost plugin cache", err, readErr)
	}
	// A cache path that cannot be created is optional, rather than activation failure.
	other := manager(t)
	if _, err := other.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other.root, ".sinan-state", "cache"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	without, err := other.Execute(context.Background(), "example.native", "echo", "cacheDir")
	if err != nil || without.Text != "" {
		t.Fatal("cache failure prevented activation", err)
	}
}
func TestFailedUpgradeRestoresPackageAndTrust(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(packageVersion(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(context.Background(), "example.native", "echo", "warm"); err != nil {
		t.Fatal(err)
	}
	candidate := packageVersion(t, true, "0.2.0")
	os.WriteFile(filepath.Join(candidate, "bad-init"), []byte("fixture"), 0600)
	if _, err := m.Upgrade(candidate, true); err == nil {
		t.Fatal("bad initialization accepted")
	}
	info := m.Snapshot().Plugins[0]
	if info.Version != "0.1.0" || !m.nativeApproved(info.ID) {
		t.Fatal("old state not restored")
	}
	if _, err := m.Execute(context.Background(), info.ID, "echo", "restored"); err != nil {
		t.Fatal("restored process unavailable", err)
	}
	mf, err := manifest.Load(filepath.Join(m.root, info.ID))
	if err != nil || mf.Version != "0.1.0" {
		t.Fatal("old files not restored")
	}
	entries, _ := filepath.Glob(filepath.Join(m.root, ".upgrade-*"))
	if len(entries) != 0 {
		t.Fatal("completed rollback left transactions")
	}
	// A failed declarative-to-native switch must not retain a new trust grant.
	if _, err := m.Install(source(t, false), false); err != nil {
		t.Fatal(err)
	}
	native, _ := manifest.Load(candidate)
	native.ID = "example.hello"
	writeManifest(t, candidate, native)
	if _, err := m.Upgrade(candidate, true); err == nil || m.nativeApproved("example.hello") {
		t.Fatal("failed conversion kept native approval")
	}
	result, err := m.Execute(context.Background(), "example.hello", "hello", "")
	if err != nil || result.Text != "你好" {
		t.Fatal("declarative package not restored", err)
	}
}
func TestRefreshDetectsChangesAndReloadIsExplicit(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, false), false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(m.root, "example.hello")
	mf, _ := manifest.Load(dir)
	mf.Contributes.Commands[0].Message = "new message"
	writeManifest(t, dir, mf)
	if !m.Refresh().Plugins[0].Changed {
		t.Fatal("same version edit not detected")
	}
	result, _ := m.Execute(context.Background(), mf.ID, "hello", "")
	if result.Text != "你好" {
		t.Fatal("refresh replaced registered commands")
	}
	if _, err := m.Reload(mf.ID); err != nil {
		t.Fatal(err)
	}
	result, _ = m.Execute(context.Background(), mf.ID, "hello", "")
	if result.Text != "new message" || m.Snapshot().Plugins[0].Changed {
		t.Fatal("reload did not apply changes")
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("invalid"), 0600)
	if !m.Refresh().Plugins[0].Changed {
		t.Fatal("invalid edit not detected")
	}
	if _, err := m.Reload(mf.ID); err == nil {
		t.Fatal("invalid reload accepted")
	}
	result, _ = m.Execute(context.Background(), mf.ID, "hello", "")
	if result.Text != "new message" {
		t.Fatal("invalid reload discarded registered commands")
	}
}
func TestReviewChecksSourceAndInstalledPackage(t *testing.T) {
	m := manager(t)
	m.Install(packageVersion(t, false, "0.1.0"), false)
	candidate := packageVersion(t, false, "0.2.0")
	preview, _ := m.PreviewPackage(candidate)
	os.WriteFile(filepath.Join(candidate, "asset.txt"), []byte("new bytes"), 0600)
	if _, err := m.ApplyPackage(candidate, false, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
		t.Fatal("changed candidate accepted")
	}
	preview, _ = m.PreviewPackage(candidate)
	if _, err := m.Upgrade(packageVersion(t, false, "0.3.0"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyPackage(candidate, false, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
		t.Fatal("stale installed baseline accepted")
	}
	if m.Snapshot().Plugins[0].Version != "0.3.0" {
		t.Fatal("stale review replaced current version")
	}
}
func TestFingerprintDetectsBinaryBytesDespiteUnchangedMetadata(t *testing.T) {
	m := manager(t)
	m.Install(source(t, true), true)
	file := filepath.Join(m.root, "example.native", "plugin")
	info, _ := os.Stat(file)
	data, _ := os.ReadFile(file)
	data[len(data)-1] ^= 1
	if err := os.WriteFile(file, data, 0700); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(file, info.ModTime(), info.ModTime())
	if !m.Refresh().Plugins[0].Changed {
		t.Fatal("same-size same-time binary edit not detected")
	}
	if _, err := m.Execute(context.Background(), "example.native", "echo", ""); err == nil || !strings.Contains(err.Error(), "重载") {
		t.Fatal("changed code activated implicitly", err)
	}
}
func TestInterruptedUpgradeRecovery(t *testing.T) {
	for _, phase := range []string{"before-replace", "after-replace", "committed"} {
		t.Run(phase, func(t *testing.T) {
			m := manager(t)
			m.Install(packageVersion(t, false, "0.1.0"), false)
			root := m.root
			m.Close()
			work, err := os.MkdirTemp(root, ".upgrade-")
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(root, "example.hello")
			if err = os.Rename(dest, filepath.Join(work, "previous")); err != nil {
				t.Fatal(err)
			}
			if phase != "before-replace" {
				os.Mkdir(dest, 0700)
				if err = copyDirectory(packageVersion(t, false, "0.2.0"), dest); err != nil {
					t.Fatal(err)
				}
			}
			m.setNativeApproval("example.hello", true)
			if err = writeUpgradeJournal(work, upgradeJournal{ID: "example.hello", Committed: phase == "committed", OldApproved: false}); err != nil {
				t.Fatal(err)
			}
			recovered, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer recovered.Close()
			want := "0.1.0"
			if phase == "committed" {
				want = "0.2.0"
			}
			if recovered.Snapshot().Plugins[0].Version != want {
				t.Fatal("wrong version after recovery")
			}
			if phase != "committed" && recovered.nativeApproved("example.hello") {
				t.Fatal("trust not rolled back")
			}
			if _, err := os.Stat(work); !os.IsNotExist(err) {
				t.Fatal("recovered transaction not cleaned")
			}
		})
	}
}

func TestFailedNativeReloadKeepsRunningInstance(t *testing.T) {
	m := manager(t)
	m.Install(source(t, true), true)
	before, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(m.root, "example.native", "bad-init")
	os.WriteFile(marker, []byte("fixture"), 0600)
	m.Refresh()
	if _, err := m.Reload("example.native"); err == nil {
		t.Fatal("invalid initialization accepted")
	}
	after, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil || after.Text != before.Text {
		t.Fatal("failed reload replaced the running instance", err)
	}
	os.Remove(marker)
	if _, err := m.Reload("example.native"); err != nil {
		t.Fatal(err)
	}
	after, err = m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil || after.Text == before.Text {
		t.Fatal("successful reload kept old process", err)
	}
}
func TestRootLeaseExcludesOtherManagersAndWaitsForCalls(t *testing.T) {
	m := manager(t)
	root := m.root
	if other, err := New(root); err == nil {
		other.Close()
		t.Fatal("second root owner accepted")
	}
	m.Install(source(t, true), true)
	pending := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		pending <- err
	}()
	awaitSearch(t, m, "example.native")
	m.Close()
	select {
	case err := <-pending:
		if err == nil {
			t.Fatal("closed runtime returned query result")
		}
	case <-time.After(time.Second):
		t.Fatal("active call did not finish after close")
	}
	other, err := New(root)
	if err != nil {
		t.Fatal("root lease not released", err)
	}
	other.Close()
}
func TestOtherPluginRemainsResponsiveDuringUpgradeInitialization(t *testing.T) {
	m := manager(t)
	m.Install(source(t, true), true)
	m.Install(source(t, false), false)
	candidate := packageVersion(t, true, "0.2.0")
	os.WriteFile(filepath.Join(candidate, "slow-start"), []byte("fixture"), 0600)
	old, _ := m.lookup("example.native")
	done := make(chan error, 1)
	go func() { _, err := m.Upgrade(candidate, true); done <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		old.mu.Lock()
		retired := old.removed
		old.mu.Unlock()
		if retired {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upgrade did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	fast := make(chan error, 1)
	go func() { _, err := m.Execute(context.Background(), "example.hello", "hello", ""); fast <- err }()
	select {
	case err := <-fast:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("unrelated command waited for upgrade")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUnrecoverableTransactionQuarantinesPackage(t *testing.T) {
	m := manager(t)
	m.Install(source(t, false), false)
	root := m.root
	m.Close()
	work, err := os.MkdirTemp(root, ".upgrade-")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(work, "previous")); err != nil {
		t.Fatal(err)
	}
	if err = writeUpgradeJournal(work, upgradeJournal{ID: "example.hello"}); err != nil {
		t.Fatal(err)
	}
	recovered, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if len(recovered.Snapshot().Plugins) != 0 || len(recovered.Snapshot().Diagnostics) == 0 {
		t.Fatal("unresolved package was discovered")
	}
	if _, err := recovered.Reload("example.hello"); err == nil {
		t.Fatal("unresolved package was reloaded")
	}
	if _, err := recovered.Upgrade(source(t, false), false); err == nil {
		t.Fatal("another transaction started before recovery")
	}
	if _, err := os.Lstat(filepath.Join(work, "previous")); err != nil {
		t.Fatal("unrecoverable backup was discarded")
	}
}
