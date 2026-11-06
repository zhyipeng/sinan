package kernel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func uiPackage(t *testing.T, native bool, version string) string {
	t.Helper()
	dir := packageVersion(t, native, version)
	if err := os.Mkdir(filepath.Join(dir, "ui"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "index.html"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	mf, err := manifest.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	mf.Contributes.UIViews = []protocol.UIView{{ID: "main", Entry: "index.html", BridgeVersion: protocol.UIBridgeVersion}}
	writeManifest(t, dir, mf)
	return dir
}
func openUI(t *testing.T, m *Manager, id string) *uiview.Session {
	t.Helper()
	s, err := m.OpenUIView(context.Background(), id, "main")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func assertUIRevoked(t *testing.T, s *uiview.Session) {
	t.Helper()
	if s.Active() {
		t.Fatal("stale UI session remains active")
	}
	if _, ok := s.Resource(s.EntryURL()); ok {
		t.Fatal("stale UI resource access accepted")
	}
	if _, err := s.Message([]byte(`{"kind":"ready","version":1}`)); err == nil {
		t.Fatal("stale UI bridge accepted")
	}
}
func TestUISessionLifecycleAndInstanceReplacement(t *testing.T) {
	for _, action := range []string{"disable", "stop", "reload", "upgrade", "uninstall", "close", "refresh-change", "open-change"} {
		t.Run(action, func(t *testing.T) {
			m := manager(t)
			source := uiPackage(t, false, "0.1.0")
			if _, err := m.Install(source, false); err != nil {
				t.Fatal(err)
			}
			s := openUI(t, m, "example.hello")
			if s.Binding().PluginID != "example.hello" || len(s.Binding().Fingerprint) != 64 {
				t.Fatal("binding missing")
			}
			var err error
			switch action {
			case "disable":
				_, err = m.SetPluginEnabled("example.hello", false)
			case "stop":
				err = m.Stop("example.hello")
			case "reload":
				_, err = m.Reload("example.hello")
			case "upgrade":
				_, err = m.Upgrade(uiPackage(t, false, "0.2.0"), false)
			case "uninstall":
				err = m.Uninstall("example.hello")
			case "close":
				m.Close()
			case "refresh-change", "open-change":
				if err := os.WriteFile(filepath.Join(m.root, "example.hello", "ui", "index.html"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
				if action == "refresh-change" {
					m.Refresh()
				} else {
					if _, openErr := m.OpenUIView(context.Background(), "example.hello", "main"); openErr == nil {
						t.Fatal("changed package opened")
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			assertUIRevoked(t, s)
			if action == "disable" {
				if _, err := m.OpenUIView(context.Background(), "example.hello", "main"); err == nil {
					t.Fatal("disabled UI opened")
				}
				m.SetPluginEnabled("example.hello", true)
				assertUIRevoked(t, s)
			}
			if action == "reload" || action == "upgrade" || action == "stop" || action == "disable" {
				fresh := openUI(t, m, "example.hello")
				if fresh == s {
					t.Fatal("old UI handle reused")
				}
				assertUIRevoked(t, s)
			}
		})
	}
}
func TestUISessionUpgradeRollbackDoesNotReviveOldHandle(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(uiPackage(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	s := openUI(t, m, "example.native")
	candidate := uiPackage(t, true, "0.2.0")
	if err := os.WriteFile(filepath.Join(candidate, "bad-init"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Upgrade(candidate, true); err == nil {
		t.Fatal("invalid initialization accepted")
	}
	assertUIRevoked(t, s)
	fresh := openUI(t, m, "example.native")
	if fresh.Binding() != s.Binding() {
		t.Fatal("rollback failed to restore original package")
	}
	assertUIRevoked(t, s)
}
func TestUISessionLimitsCancellationAndConcurrentDisable(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(uiPackage(t, false, "0.1.0"), false); err != nil {
		t.Fatal(err)
	}
	sessions := []*uiview.Session{}
	for i := 0; i < maxUIPluginSessions; i++ {
		sessions = append(sessions, openUI(t, m, "example.hello"))
	}
	if _, err := m.OpenUIView(context.Background(), "example.hello", "main"); err == nil {
		t.Fatal("per-plugin limit ignored")
	}
	sessions[0].Revoke()
	fresh := openUI(t, m, "example.hello")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.OpenUIView(ctx, "example.hello", "main"); err == nil {
		t.Fatal("cancelled open accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				fresh.Resource(fresh.EntryURL())
			}
		})
	}
	if _, err := m.SetPluginEnabled("example.hello", false); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	assertUIRevoked(t, fresh)
}

func TestUISessionFailedPolicyWritePreservesSession(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(uiPackage(t, false, "0.1.0"), false); err != nil {
		t.Fatal(err)
	}
	s := openUI(t, m, "example.hello")
	if err := os.Mkdir(filepath.Join(m.root, ".sinan-state", "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPluginEnabled("example.hello", false); err == nil {
		t.Fatal("write failure ignored")
	}
	if !s.Active() {
		t.Fatal("failed policy write revoked UI")
	}
}

func TestUISessionFailedReloadRevokesChangedPackage(t *testing.T) {
	m := manager(t)
	if _, err := m.Install(uiPackage(t, true, "0.1.0"), true); err != nil {
		t.Fatal(err)
	}
	s := openUI(t, m, "example.native")
	if err := os.WriteFile(filepath.Join(m.root, "example.native", "bad-init"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reload("example.native"); err == nil {
		t.Fatal("invalid initialization accepted")
	}
	assertUIRevoked(t, s)
}

func TestUISessionGlobalLimitAndClosedSlotReuse(t *testing.T) {
	m := manager(t)
	var first *uiview.Session
	for i := 0; i < 5; i++ {
		dir := uiPackage(t, false, "0.1.0")
		mf, err := manifest.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		mf.ID = fmt.Sprintf("example.slot-%d", i)
		writeManifest(t, dir, mf)
		if _, err = m.Install(dir, false); err != nil {
			t.Fatal(err)
		}
		if i < 4 {
			for j := 0; j < maxUIPluginSessions; j++ {
				s := openUI(t, m, mf.ID)
				if first == nil {
					first = s
				}
			}
		} else {
			if _, err = m.OpenUIView(context.Background(), mf.ID, "main"); err == nil {
				t.Fatal("global UI limit ignored")
			}
			first.Revoke()
			openUI(t, m, mf.ID)
		}
	}
}

func TestUIOpenRequiresDeclaredViewID(t *testing.T) {
	m := manager(t)
	source := uiPackage(t, false, "0.1.0")
	if err := os.WriteFile(filepath.Join(source, "ui", "other.html"), []byte("unlisted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(source, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "index.html", "other.html", "ui/index.html", "../main", "settings", "unknown"} {
		if _, err := m.OpenUIView(context.Background(), "example.hello", id); err == nil {
			t.Fatal("undeclared view accepted")
		}
	}
	s := openUI(t, m, "example.hello")
	if s.EntryURL() != "sinan-plugin://example.hello/index.html" || s.Binding().ViewID != "main" || s.Binding().BridgeVersion != "1" {
		t.Fatal("wrong declared view binding")
	}
	// Removing the declaration on upgrade invalidates the old handle and cannot
	// be bypassed by asking for the former resource path.
	next := uiPackage(t, false, "0.2.0")
	mf, err := manifest.Load(next)
	if err != nil {
		t.Fatal(err)
	}
	mf.Contributes.UIViews = nil
	writeManifest(t, next, mf)
	if _, err = m.Upgrade(next, false); err != nil {
		t.Fatal(err)
	}
	assertUIRevoked(t, s)
	if _, err = m.OpenUIView(context.Background(), "example.hello", "main"); err == nil {
		t.Fatal("removed declaration remained available")
	}
}

func TestUIWithoutDeclarationAndMissingResourceDenied(t *testing.T) {
	for _, mode := range []string{"no-declaration", "missing-resource"} {
		t.Run(mode, func(t *testing.T) {
			m := manager(t)
			source := uiPackage(t, false, "0.1.0")
			mf, err := manifest.Load(source)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "no-declaration" {
				mf.Contributes.UIViews = nil
			} else {
				mf.Contributes.UIViews[0].Entry = "missing.html"
			}
			writeManifest(t, source, mf)
			if _, err = m.Install(source, false); err != nil {
				t.Fatal(err)
			}
			if _, err = m.OpenUIView(context.Background(), "example.hello", "main"); err == nil {
				t.Fatal("unavailable view opened")
			}
			if len(m.views) != 0 {
				t.Fatal("failed view occupied a slot")
			}
		})
	}
}

func TestUICommandResolvesInstalledBindingWithoutBackend(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			m := manager(t)
			source := uiPackage(t, native, "0.1.0")
			mf, err := manifest.Load(source)
			if err != nil {
				t.Fatal(err)
			}
			mf.Contributes.Commands = append(mf.Contributes.Commands, manifest.Command{ID: "window", Title: "Window", UIView: "main"})
			writeManifest(t, source, mf)
			if _, err = m.Install(source, native); err != nil {
				t.Fatal(err)
			}
			if err = m.Stop(mf.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = m.Execute(context.Background(), mf.ID, "window", ""); err == nil {
				t.Fatal("UI command executed as backend command")
			}
			s, err := m.OpenUICommand(context.Background(), mf.ID, "window")
			if err != nil {
				t.Fatal(err)
			}
			if s.Binding().ViewID != "main" {
				t.Fatal("wrong command binding")
			}
			p, err := m.lookup(mf.ID)
			if err != nil {
				t.Fatal(err)
			}
			p.mu.Lock()
			running := p.process != nil
			p.mu.Unlock()
			if running {
				t.Fatal("opening UI activated backend")
			}
			for _, commandID := range []string{"main", "index.html", "missing", mf.Contributes.Commands[0].ID} {
				if _, err = m.OpenUICommand(context.Background(), mf.ID, commandID); err == nil {
					t.Fatal("non-UI command or view path accepted")
				}
			}
			if _, err = m.SetPluginEnabled(mf.ID, false); err != nil {
				t.Fatal(err)
			}
			assertUIRevoked(t, s)
			if _, err = m.OpenUICommand(context.Background(), mf.ID, "window"); err == nil {
				t.Fatal("disabled UI command opened")
			}
			if _, err = m.SetPluginEnabled(mf.ID, true); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(m.root, mf.ID, "ui", "index.html"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = m.OpenUICommand(context.Background(), mf.ID, "window"); err == nil {
				t.Fatal("changed package UI command opened")
			}
		})
	}
}

func TestUICommandUsesBindingAfterUpgrade(t *testing.T) {
	m := manager(t)
	prepare := func(version, viewID string) string {
		dir := uiPackage(t, false, version)
		mf, err := manifest.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		mf.Contributes.UIViews = append(mf.Contributes.UIViews, protocol.UIView{ID: "second", Entry: "second.html", BridgeVersion: "1"})
		mf.Contributes.Commands = append(mf.Contributes.Commands, manifest.Command{ID: "window", Title: "Window", UIView: viewID})
		writeManifest(t, dir, mf)
		if err = os.WriteFile(filepath.Join(dir, "ui", "second.html"), []byte("second"), 0600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	if _, err := m.Install(prepare("0.1.0", "main"), false); err != nil {
		t.Fatal(err)
	}
	old, err := m.OpenUICommand(context.Background(), "example.hello", "window")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Upgrade(prepare("0.2.0", "second"), false); err != nil {
		t.Fatal(err)
	}
	assertUIRevoked(t, old)
	current, err := m.OpenUICommand(context.Background(), "example.hello", "window")
	if err != nil {
		t.Fatal(err)
	}
	if current.Binding().ViewID != "second" || current.EntryURL() != "sinan-plugin://example.hello/second.html" {
		t.Fatal("command used stale view from before upgrade")
	}
}

func TestUIViewMetadataDoesNotConsumeSlots(t *testing.T) {
	m := manager(t)
	source := uiPackage(t, false, "0.1.0")
	mf, err := manifest.Load(source)
	if err != nil {
		t.Fatal(err)
	}
	mf.Contributes.UIViews[0].Title = "输入"
	mf.Contributes.Commands = append(mf.Contributes.Commands, manifest.Command{ID: "window", Title: "Window", UIView: "main"})
	writeManifest(t, source, mf)
	if _, err = m.Install(source, false); err != nil {
		t.Fatal(err)
	}
	for range maxUIPluginSessions {
		openUI(t, m, mf.ID)
	}
	for range 8 {
		binding, err := m.ResolveUICommand(context.Background(), mf.ID, "window")
		if err != nil || binding.Title != mf.Name+" · 输入" || binding.ViewID != "main" {
			t.Fatal("metadata resolution failed at the session limit")
		}
	}
	if _, err = m.OpenUIView(context.Background(), mf.ID, "main"); err == nil {
		t.Fatal("metadata resolution changed the session limit")
	}
	if _, err = m.SetPluginEnabled(mf.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ResolveUIView(context.Background(), mf.ID, "main"); err == nil {
		t.Fatal("disabled view metadata reused")
	}
}
