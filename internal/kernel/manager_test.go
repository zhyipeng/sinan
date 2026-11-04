package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

var helper []byte

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sinan-native-test-")
	if err != nil {
		os.Exit(1)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "plugin"), "./testdata/plugin")
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		os.Exit(1)
	}
	helper, err = os.ReadFile(filepath.Join(dir, "plugin"))
	if err != nil {
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func source(t *testing.T, native bool) string {
	t.Helper()
	dir := t.TempDir()
	mf := manifest.Manifest{ID: "example.hello", Name: "Hello", Version: "0.1.0", APIVersion: "1", Runtime: manifest.Runtime{Kind: "declarative"}, Contributes: manifest.Contributions{Commands: []manifest.Command{{ID: "hello", Title: "Hello", Message: "你好"}}}}
	if native {
		mf.ID = "example.native"
		mf.Contributes.SearchProviders = []manifest.SearchProvider{{ID: "demo"}}
		mf.Runtime = manifest.Runtime{Kind: "process", Entry: "plugin"}
		mf.Contributes.Commands = []manifest.Command{{ID: "echo", Title: "Echo"}, {ID: "hang", Title: "Hang"}, {ID: "crash", Title: "Crash"}, {ID: "invalid", Title: "Invalid"}}
		if err := os.WriteFile(filepath.Join(dir, "plugin"), helper, 0700); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func manager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestDeclarativeLifecycle(t *testing.T) {
	m := manager(t)
	dir := source(t, false)
	if _, err := m.Install(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(dir, false); err == nil {
		t.Fatal("duplicate install succeeded")
	}
	result, err := m.Execute(context.Background(), "example.hello", "hello", "")
	if err != nil || result.Text != "你好" {
		t.Fatalf("execute: %v %#v", err, result)
	}
	if err := m.Stop("example.hello"); err != nil {
		t.Fatal(err)
	}
	if got := m.Snapshot().Plugins[0].State; got != "deactivated" {
		t.Fatal(got)
	}
	if err := m.Uninstall("example.hello"); err != nil {
		t.Fatal(err)
	}
	if len(m.Refresh().Plugins) != 0 {
		t.Fatal("plugin still installed")
	}
	if _, err := m.Execute(context.Background(), "example.hello", "hello", ""); err == nil {
		t.Fatal("uninstalled command executed")
	}
}

func TestNativeLifecycleAndRediscovery(t *testing.T) {
	m := manager(t)
	dir := source(t, true)
	if _, err := m.Install(dir, false); err == nil {
		t.Fatal("untrusted native installation succeeded")
	}
	if _, err := m.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := m.Execute(context.Background(), "example.native", "echo", "跨进程回声")
		if err != nil || got.Text != "跨进程回声" {
			t.Fatalf("%v: %#v", err, got)
		}
		if m.Snapshot().Plugins[0].State != "running" {
			t.Fatal("not running")
		}
		if err := m.Stop("example.native"); err != nil {
			t.Fatal(err)
		}
	}
	m.Close()
	other, err := New(m.root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Execute(context.Background(), "example.native", "echo", "rediscovered"); err != nil {
		t.Fatal(err)
	}
	if err := other.Uninstall("example.native"); err != nil {
		t.Fatal(err)
	}
	if len(other.Refresh().Plugins) != 0 {
		t.Fatal("removed plugin survived refresh")
	}
}

func TestNativeFailuresDoNotBreakHost(t *testing.T) {
	for _, command := range []string{"hang", "crash", "invalid"} {
		t.Run(command, func(t *testing.T) {
			m := manager(t)
			if _, err := m.Install(source(t, true), true); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Install(source(t, false), false); err != nil {
				t.Fatal(err)
			}
			// This case tests a running handler, not macOS cold-start/signature latency.
			if _, err := m.Execute(context.Background(), "example.native", "echo", "warm"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			if _, err := m.Execute(ctx, "example.native", command, ""); err == nil {
				t.Fatal("broken plugin succeeded")
			} else if command == "hang" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("unresponsive plugin did not time out", err)
			}
			if m.plugins["example.native"].state != "failed" {
				t.Fatal("failure not recorded")
			}
			got, err := m.Execute(context.Background(), "example.hello", "hello", "")
			if err != nil || got.Text != "你好" {
				t.Fatal("host did not recover")
			}
			if _, err := m.Execute(context.Background(), "example.native", "echo", "restart"); err != nil {
				t.Fatal("plugin restart failed", err)
			}
		})
	}
}

func TestInstallRejectsSymlinksAndPathTraversal(t *testing.T) {
	m := manager(t)
	dir := source(t, false)
	if err := os.Symlink(filepath.Join(dir, "manifest.json"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(dir, false); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := m.Uninstall("../outside"); err == nil {
		t.Fatal("invalid id accepted")
	}
	if len(m.Snapshot().Plugins) != 0 {
		t.Fatal("failed installation registered")
	}
}

func TestMalformedPluginDoesNotPreventStartup(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "example.broken"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if len(m.Snapshot().Diagnostics) != 1 {
		t.Fatal("missing diagnostic")
	}
}

func TestDirectNativeDiscoveryDoesNotGrantTrust(t *testing.T) {
	m := manager(t)
	dir := filepath.Join(m.root, "example.native")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyDirectory(source(t, true), dir); err != nil {
		t.Fatal(err)
	}
	m.Refresh()
	if _, err := m.Execute(context.Background(), "example.native", "echo", ""); err == nil {
		t.Fatal("unapproved native code ran")
	}
}

func TestConcurrentSnapshotAndCommands(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_ = m.Snapshot()
			if _, err := m.Execute(context.Background(), "example.native", "echo", "concurrent"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

func awaitSearch(t *testing.T, m *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result, err := m.Execute(context.Background(), id, "echo", "active")
		if err != nil {
			t.Fatal(err)
		}
		if result.Text != "0" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("search did not start")
}

func TestCooperativeSearchCancellationPreservesCommandAndProcess(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	before, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queryDone := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(ctx, "example.native", protocol.SearchParams{ProviderID: "demo", Query: "slow", Limit: 20})
		queryDone <- err
	}()
	awaitSearch(t, m, "example.native")
	commandDone := make(chan error, 1)
	go func() {
		_, err := m.Execute(context.Background(), "example.native", "echo", "delay")
		commandDone <- err
	}()
	cancel()
	if err := <-queryDone; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := <-commandDone; err != nil {
		t.Fatal("unrelated command interrupted", err)
	}
	// A second search proves the server continues dispatching after cancellation.
	if _, err := m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "after", Limit: 20}); err != nil {
		t.Fatal(err)
	}
	// Wait past the cancellation grace to detect a wrongly retained watchdog.
	time.Sleep(550 * time.Millisecond)
	after, err := m.Execute(context.Background(), "example.native", "echo", "pid")
	if err != nil || after.Text != before.Text {
		t.Fatalf("cooperating process was killed: %v", err)
	}
}

func TestOtherPluginAndSnapshotRemainResponsiveDuringSearch(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(source(t, false), false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(ctx, "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		done <- err
	}()
	awaitSearch(t, m, "example.native")
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = m.Snapshot()
		if _, err := m.Execute(context.Background(), "example.hello", "hello", ""); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-finished:
	case <-time.After(400 * time.Millisecond):
		t.Fatal("global registry blocked by search")
	}
	cancel()
	<-done
}

func TestUninstallInvalidatesPendingSearch(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		done <- err
	}()
	awaitSearch(t, m, "example.native")
	if err := m.Uninstall("example.native"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("removed provider returned results")
	}
	if len(m.Snapshot().Plugins) != 0 {
		t.Fatal("removed plugin revived")
	}
}

func TestNonCooperatingSearchIsReaped(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(ctx, "example.native", protocol.SearchParams{ProviderID: "demo", Query: "ignore", Limit: 20})
		done <- err
	}()
	awaitSearch(t, m, "example.native")
	cancel()
	<-done
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.Snapshot().Plugins[0].State == "failed" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("non-cooperating process was not stopped")
}

func TestIndependentNativeProvidersRunInParallel(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	second := source(t, true)
	mf, err := manifest.Load(second)
	if err != nil {
		t.Fatal(err)
	}
	mf.ID = "example.second"
	encoded, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(second, "manifest.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(second, true); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"example.native", "example.second"} {
		if _, err := m.Execute(context.Background(), id, "echo", "warm"); err != nil {
			t.Fatal(err)
		}
	}
	slowCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(slowCtx, "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		done <- err
	}()
	awaitSearch(t, m, "example.native")
	fastCtx, fastCancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer fastCancel()
	result, err := m.QueryProvider(fastCtx, "example.second", protocol.SearchParams{ProviderID: "demo", Query: "fast", Limit: 20})
	if err != nil || len(result.Items) != 1 {
		t.Fatal("independent provider blocked", err)
	}
	cancel()
	<-done
}

func TestActivationTimeoutPreservesContextError(t *testing.T) {
	m := manager(t)
	dir := source(t, true)
	if err := os.WriteFile(filepath.Join(dir, "slow-start"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := m.QueryProvider(ctx, "example.native", protocol.SearchParams{ProviderID: "demo", Query: "query", Limit: 20})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("activation timeout was misclassified: %v", err)
	}
}

func TestOnlySuccessfulCommandsRecordUsage(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, false), false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(context.Background(), "example.hello", "missing", "input"); err == nil {
		t.Fatal("expected failure")
	}
	if m.UsageBoost("example.hello", "missing", "input") != 0 {
		t.Fatal("failed action affected ranking")
	}
	if _, err := m.Execute(context.Background(), "example.hello", "hello", "input"); err != nil {
		t.Fatal(err)
	}
	if m.UsageBoost("example.hello", "hello", "input") <= 0 {
		t.Fatal("successful action not recorded")
	}
	m.Close()
	other, err := New(m.root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if other.UsageBoost("example.hello", "hello", "input") <= 0 {
		t.Fatal("history did not survive manager restart")
	}
	if err := other.ClearUsage(); err != nil {
		t.Fatal(err)
	}
	if other.UsageBoost("example.hello", "hello", "input") != 0 {
		t.Fatal("history did not clear")
	}
}

func TestSettingsReinitializeOnlyAffectedPlugin(t *testing.T) {
	m := manager(t)
	dir := source(t, true)
	mf, err := manifest.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	mf.Contributes.Settings = []manifest.Setting{{ID: "directories", Title: "Directories", Type: "directory-list"}}
	data, _ := json.Marshal(mf)
	os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
	if _, err = m.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	otherDir := source(t, true)
	otherManifest, _ := manifest.Load(otherDir)
	otherManifest.ID = "example.other"
	otherData, _ := json.Marshal(otherManifest)
	os.WriteFile(filepath.Join(otherDir, "manifest.json"), otherData, 0600)
	if _, err := m.Install(otherDir, true); err != nil {
		t.Fatal(err)
	}
	unrelatedBefore, err := m.Execute(context.Background(), "example.other", "echo", "pid")
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.Execute(context.Background(), mf.ID, "echo", "pid")
	if err != nil {
		t.Fatal(err)
	}
	pending := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(context.Background(), mf.ID, protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		pending <- err
	}()
	awaitSearch(t, m, mf.ID)
	target := t.TempDir()
	saved, err := m.SavePluginSettings(mf.ID, 0, map[string][]string{"directories": {target, target}})
	if err != nil || saved.Revision != 1 || len(saved.Values["directories"]) != 1 {
		t.Fatal("settings not normalized", err)
	}
	select {
	case pendingErr := <-pending:
		if pendingErr == nil {
			t.Fatal("retired process search succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("settings change left pending search hanging")
	}
	unrelatedAfter, err := m.Execute(context.Background(), "example.other", "echo", "pid")
	if err != nil || unrelatedBefore.Text != unrelatedAfter.Text {
		t.Fatal("unrelated plugin was restarted")
	}
	after, err := m.Execute(context.Background(), mf.ID, "echo", "pid")
	if err != nil || after.Text == before.Text {
		t.Fatal("plugin process not refreshed", err)
	}
	result, err := m.Execute(context.Background(), mf.ID, "echo", "settings")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string][]string
	json.Unmarshal([]byte(result.Text), &values)
	if len(values["directories"]) != 1 || values["directories"][0] != target {
		t.Fatal("initialize did not receive own settings")
	}
	for _, invalid := range []map[string][]string{{"unknown": {}}, {"directories": {"relative/path"}}, {"directories": {filepath.Join(target, "missing")}}} {
		if _, err := m.SavePluginSettings(mf.ID, 1, invalid); err == nil {
			t.Fatal("invalid settings accepted")
		}
	}
	if _, err := m.SavePluginSettings(mf.ID, 0, map[string][]string{}); err == nil {
		t.Fatal("stale writer accepted")
	}
	m.Close()
	other, err := New(m.root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	loaded, err := other.PluginSettings(mf.ID)
	if err != nil || loaded.Revision != 1 {
		t.Fatal("settings lost across restart", err)
	}
	// Removing the plugin does not delete the host's configuration service.
	if err := other.Uninstall(mf.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Install(dir, true); err != nil {
		t.Fatal(err)
	}
	result, err = other.Execute(context.Background(), mf.ID, "echo", "settings")
	if err != nil || !strings.Contains(result.Text, target) {
		t.Fatal("reinstalled plugin lost settings", err)
	}
}
