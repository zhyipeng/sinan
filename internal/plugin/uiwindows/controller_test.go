package uiwindows

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiprocess"
	protocol "sinan/packages/plugin-protocol"
)

var guest, nativeGuest string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sinan-ui-windows-test-")
	if err != nil {
		os.Exit(1)
	}
	guest = filepath.Join(dir, "guest")
	if exec.Command("go", "build", "-o", guest, "../uiprocess/testdata/guest").Run() != nil {
		os.RemoveAll(dir)
		os.Exit(1)
	}
	if os.Getenv("SINAN_NATIVE_UI_TEST") == "1" && runtime.GOOS == "darwin" {
		nativeGuest = filepath.Join(dir, "sinan-ui-host")
		if exec.Command("go", "build", "-o", nativeGuest, "../../../apps/ui-host").Run() != nil {
			os.RemoveAll(dir)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func installed(t *testing.T, script string) *kernel.Manager {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "ui"), 0700); err != nil {
		t.Fatal(err)
	}
	entry := []byte("fixture")
	if script != "" {
		entry = []byte(`<!doctype html><html><head><meta charset="utf-8"></head><body><input autofocus id="input"><script src="main.js"></script></body></html>`)
	}
	if err := os.WriteFile(filepath.Join(source, "ui", "index.html"), entry, 0600); err != nil {
		t.Fatal(err)
	}
	if script != "" {
		if err := os.WriteFile(filepath.Join(source, "ui", "main.js"), []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mf := manifest.Manifest{ID: "example.ui", Name: "Fixture", Version: "0.1.0", APIVersion: "1", Runtime: manifest.Runtime{Kind: "declarative"}, Contributes: manifest.Contributions{Commands: []manifest.Command{{ID: "hello", Title: "Fixture", Message: "fixture"}, {ID: "window", Title: "Window", UIView: "main"}, {ID: "alias", Title: "Alias", UIView: "main"}}, UIViews: []protocol.UIView{{ID: "main", Entry: "index.html", BridgeVersion: "1"}}}}
	data, err := json.Marshal(mf)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := kernel.New(filepath.Join(root, "installed"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err = manager.Install(source, false); err != nil {
		t.Fatal(err)
	}
	return manager
}

func controller(t *testing.T, m *kernel.Manager, mode string) (*Controller, chan Event) {
	t.Helper()
	events := make(chan Event, 8)
	c, err := New(m, guest, func(e Event) { events <- e })
	if err != nil {
		t.Fatal(err)
	}
	c.options.Args = []string{mode}
	c.options.StartupTimeout = 3 * time.Second
	c.options.ShutdownGrace = 100 * time.Millisecond
	t.Cleanup(c.Close)
	return c, events
}

func waitEvent(t *testing.T, events chan Event, timeout time.Duration) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(timeout):
		t.Fatal("guest did not finish")
		return Event{}
	}
}

func TestControllerLifecycle(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "wait-revoke")
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	if err := c.Open("example.ui", "main"); err == nil {
		t.Fatal("duplicate guest accepted")
	}
	if _, err := m.SetPluginEnabled("example.ui", false); err != nil {
		t.Fatal(err)
	}
	if event := waitEvent(t, events, 3*time.Second); event.Failed || event.ViewID != "main" {
		t.Fatal("cooperative revoke failed")
	}
	if err := c.Open("example.ui", "main"); err == nil {
		t.Fatal("disabled plugin opened")
	}
	if _, err := m.SetPluginEnabled("example.ui", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("example.ui"); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("stop failed")
	}
}

func TestControllerFailureAndReopen(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "ready-crash")
	if err := c.Open("example.ui", "missing"); err == nil {
		t.Fatal("undeclared view opened")
	}
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	if !waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("crash reported success")
	}
	c.options.Args = []string{"normal"}
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("healthy guest affected")
	}
}

func TestCommandAndManagementShareWindowReservation(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "wait-revoke")
	if err := c.OpenCommand("example.ui", "hello"); err == nil {
		t.Fatal("non-UI command opened")
	}
	if err := c.OpenCommand("example.ui", "window"); err != nil {
		t.Fatal(err)
	}
	for _, open := range []func() error{
		func() error { return c.Open("example.ui", "main") },
		func() error { return c.OpenCommand("example.ui", "alias") },
	} {
		if open() == nil {
			t.Fatal("same view opened through another entry")
		}
	}
	select {
	case <-events:
		t.Fatal("rejected duplicate revoked the original window")
	default:
	}
	if err := m.Stop("example.ui"); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("cooperative original guest failed")
	}
	if err := c.OpenCommand("example.ui", "alias"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("example.ui"); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("command alias could not reopen closed window")
	}
}

func TestControllerShutdownDuringStartup(t *testing.T) {
	c, _ := controller(t, installed(t, ""), "silent")
	finished := make(chan struct{})
	go func() { _ = c.Open("example.ui", "main"); close(finished) }()
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		started := c.opening["example.ui/main"]
		c.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(c.Close)
	}
	wg.Wait()
	<-finished
	if err := c.Open("example.ui", "main"); err == nil {
		t.Fatal("closed controller accepted guest")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.opening) != 0 {
		t.Fatal("shutdown leaked reservation")
	}
}

func TestControllerRejectsHelperAliases(t *testing.T) {
	m := installed(t, "")
	for _, name := range []string{"relative", filepath.Join(t.TempDir(), "missing")} {
		if c, err := New(m, name, nil); err == nil {
			c.Close()
			t.Fatal("invalid helper accepted")
		}
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(guest, alias); err != nil {
		t.Fatal(err)
	}
	if c, err := New(m, alias, nil); err == nil {
		c.Close()
		t.Fatal("helper symlink accepted")
	}
}

func TestExitNotificationCanCloseController(t *testing.T) {
	m := installed(t, "")
	finished := make(chan struct{})
	var c *Controller
	var err error
	c, err = New(m, guest, func(Event) { c.Close(); close(finished) })
	if err != nil {
		t.Fatal(err)
	}
	c.options.Args = []string{"normal"}
	t.Cleanup(c.Close)
	if err = c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("reaping waited for exit notification")
	}
}

func TestNativeHost(t *testing.T) {
	if nativeGuest == "" {
		t.Skip("set SINAN_NATIVE_UI_TEST=1 on macOS to open a real WebView")
	}
	// The production host must not invoke probe functions or accept probe-only
	// messages. Keep it alive beyond the probe's old eight-second deadline.
	m := installed(t, `
const send = body => window.webkit.messageHandlers.sinan.postMessage(body);
window.runProbe = () => { throw new Error('must not invoke diagnostic code'); };
if (typeof window._wails !== 'undefined' || typeof window.__WAILS__ !== 'undefined') throw new Error('unexpected bridge');
if (window.confirm('fixed native dialog test') !== false) throw new Error('dialog permitted');
send({kind:'ready',version:1});
send({kind:'report',report:{bindingsAbsent:true,storageIsolated:true,ownResourceLoaded:true,networkBlocked:true}});
send({kind:'revoke'});
setTimeout(() => {
  if (document.activeElement.id === 'input') send({kind:'close'});
}, 8300);
`)
	events := make(chan Event, 1)
	c, err := New(m, nativeGuest, func(e Event) { events <- e })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	started := time.Now()
	if err = c.OpenCommand("example.ui", "window"); err != nil {
		t.Fatal(err)
	}
	if event := waitEvent(t, events, 12*time.Second); event.Failed || time.Since(started) < 8*time.Second {
		t.Fatal("interactive native mode failed")
	}
	if m.Snapshot().Plugins[0].Disabled {
		t.Fatal("probe action affected plugin")
	}
}

func TestNativeHostDispatchesAppKitEvents(t *testing.T) {
	if nativeGuest == "" {
		t.Skip("set SINAN_NATIVE_UI_TEST=1 on macOS")
	}
	m := installed(t, `
const send = body => window.webkit.messageHandlers.sinan.postMessage(body);
send({kind:'ready',version:1});
setTimeout(()=>send({kind:'close'}),250);
`)
	session, err := m.OpenUIView(context.Background(), "example.ui", "main")
	if err != nil {
		t.Fatal(err)
	}
	process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{Executable: nativeGuest, Lifetime: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	result := process.Wait()
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var report struct {
		EventLoopResponsive bool `json:"eventLoopResponsive"`
		CloseReceived       bool `json:"closeReceived"`
	}
	if json.Unmarshal(result.Report, &report) != nil || !report.EventLoopResponsive || !report.CloseReceived {
		t.Fatal("script completed without dispatching the native AppKit queue")
	}
}
