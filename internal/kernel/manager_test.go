package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
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
	if len(m.Refresh().Plugins) != 0 {
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
