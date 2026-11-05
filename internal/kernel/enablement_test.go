package kernel

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

func TestDisableStopsInflightRequestsAndPersistsAcrossRestart(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(context.Background(), "example.native", "echo", "pid"); err != nil {
		t.Fatal(err)
	}
	p, _ := m.lookup("example.native")
	p.mu.Lock()
	oldProcess := p.process
	p.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "block", Limit: 20})
		done <- err
	}()
	awaitSearch(t, m, "example.native")
	info, err := m.SetPluginEnabled("example.native", false)
	if err != nil || !info.Disabled || info.State != "disabled" || oldProcess.Alive() {
		t.Fatal("disable failed to stop process", err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("disabled query returned a late result")
		}
	case <-time.After(time.Second):
		t.Fatal("disabled query hung")
	}
	if _, err = m.Execute(context.Background(), "example.native", "echo", "pid"); err == nil {
		t.Fatal("disabled command activated plugin")
	}
	if _, err = m.QueryProvider(context.Background(), "example.native", protocol.SearchParams{ProviderID: "demo", Query: "query", Limit: 20}); err == nil {
		t.Fatal("disabled provider activated plugin")
	}
	if err = m.Stop("example.native"); err != nil {
		t.Fatal(err)
	}
	if !m.Refresh().Plugins[0].Disabled {
		t.Fatal("stop/refresh cleared disabled flag")
	}
	root := m.root
	m.Close()
	next, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if !next.Snapshot().Plugins[0].Disabled {
		t.Fatal("restart forgot disabled state")
	}
	if _, err = next.Execute(context.Background(), "example.native", "echo", "pid"); err == nil {
		t.Fatal("restart activated disabled plugin")
	}
	info, err = next.SetPluginEnabled("example.native", true)
	if err != nil || info.Disabled || info.State != "installed" {
		t.Fatal("enable failed", err)
	}
	p, _ = next.lookup("example.native")
	if p.process != nil {
		t.Fatal("enable should leave activation lazy")
	}
	if _, err = next.Execute(context.Background(), "example.native", "echo", "pid"); err != nil {
		t.Fatal("re-enabled plugin did not activate", err)
	}
}

func TestDisabledNativeUpgradeAndReloadDoNotExecuteCode(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(packageVersion(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPluginEnabled("example.native", false); err != nil {
		t.Fatal(err)
	}
	source := packageVersion(t, true, "0.2.0")
	if err := os.WriteFile(filepath.Join(source, "bad-init"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := m.Upgrade(source, true)
	if err != nil || !info.Disabled || info.Version != "0.2.0" {
		t.Fatal("disabled upgrade initialized plugin or lost state", err)
	}
	info, err = m.Reload("example.native")
	if err != nil || !info.Disabled || info.State != "disabled" {
		t.Fatal("disabled reload initialized plugin", err)
	}
	p, _ := m.lookup("example.native")
	if p.process != nil {
		t.Fatal("disabled package started")
	}
	if _, err = m.SetPluginEnabled("example.native", true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Execute(context.Background(), "example.native", "echo", "pid"); err == nil {
		t.Fatal("bad-init fixture not exercised")
	}
	if _, err = m.SetPluginEnabled("example.native", false); err != nil {
		t.Fatal(err)
	}
	if err = m.Uninstall("example.native"); err != nil {
		t.Fatal(err)
	}
	info, err = m.Install(source, true)
	if err != nil || !info.Disabled {
		t.Fatal("reinstall forgot explicit disabled choice", err)
	}
}

func TestEnablementWriteFailureLeavesLiveInstanceAndPolicyUnchanged(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(source(t, true), true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Execute(context.Background(), "example.native", "echo", "pid"); err != nil {
		t.Fatal(err)
	}
	p, _ := m.lookup("example.native")
	p.mu.Lock()
	before := p.process
	p.mu.Unlock()
	if err := os.Mkdir(filepath.Join(m.root, ".sinan-state", "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPluginEnabled("example.native", false); err == nil {
		t.Fatal("write failure ignored")
	}
	p.mu.Lock()
	unchanged := p.process == before && !p.disabled
	p.mu.Unlock()
	if !unchanged || !before.Alive() || m.settings.Get(enablementNamespace).Revision != 0 {
		t.Fatal("failed save changed live policy")
	}
}

func TestConcurrentEnablementChangesDoNotLoseOtherPlugins(t *testing.T) {
	m := manager(t)
	for _, native := range []bool{false, true} {
		if _, err := m.Install(source(t, native), native); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"example.hello", "example.native"} {
		wg.Go(func() { _, err := m.SetPluginEnabled(id, false); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(m.settings.Get(enablementNamespace).Values["disabled"]) != 2 {
		t.Fatal("concurrent update lost a disabled plugin")
	}
	if _, err := m.SetPluginEnabled("example.hello", true); err != nil {
		t.Fatal(err)
	}
	for _, p := range m.Snapshot().Plugins {
		if p.Disabled != (p.ID == "example.native") {
			t.Fatal("enabling one plugin changed another")
		}
	}
	if _, err := m.PluginSettings(enablementNamespace); err == nil {
		t.Fatal("host policy exposed as plugin settings")
	}
}

func TestUnreadableOrInvalidEnablementNeverReactivatesPlugins(t *testing.T) {
	for _, data := range []string{`{broken`, `{"version":1,"plugins":{"$plugins":{"revision":1,"values":{"disabled":["../invalid"]}}}}`} {
		t.Run(data[:2], func(t *testing.T) {
			m := manager(t)
			if _, err := m.Install(source(t, false), false); err != nil {
				t.Fatal(err)
			}
			root := m.root
			m.Close()
			path := filepath.Join(root, ".sinan-state", "settings.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			next, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			if snapshot := next.Snapshot(); !snapshot.Plugins[0].Disabled || len(snapshot.Diagnostics) == 0 {
				t.Fatal("invalid policy defaulted to enabled")
			}
			if _, err := next.Execute(context.Background(), "example.hello", "hello", ""); err == nil {
				t.Fatal("invalid policy allowed execution")
			}
			if _, err := next.SetPluginEnabled("example.hello", true); err == nil {
				t.Fatal("invalid policy overwritten")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, []byte(data)) {
				t.Fatal("invalid file not preserved", err)
			}
		})
	}
}
