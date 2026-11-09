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
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
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
	// Occupy the remaining kernel slots. Focusing must not allocate a session.
	for range 3 {
		s, err := m.OpenUIView(context.Background(), "example.ui", "main")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Revoke)
	}
	c.mu.Lock()
	originalPID := c.opening["example.ui/main"].process.PID()
	c.mu.Unlock()
	for _, open := range []func() error{
		func() error { return c.Open("example.ui", "main") },
		func() error { return c.OpenCommand("example.ui", "alias") },
	} {
		if err := open(); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	currentPID := c.opening["example.ui/main"].process.PID()
	c.mu.Unlock()
	if originalPID != currentPID {
		t.Fatal("focus created a different process")
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
		if started != nil {
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

func TestConcurrentStartupWaitersShareWindow(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "wait-revoke")
	const count = 6
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() { errors <- c.OpenCommand("example.ui", "window") })
	}
	wg.Wait()
	close(errors)
	// Calls that observe startup share it; calls arriving after startup may
	// encounter the single outstanding focus control, but never spawn a guest.
	succeeded := 0
	for err := range errors {
		if err == nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		t.Fatal("no startup succeeded")
	}
	c.mu.Lock()
	windows := len(c.opening)
	c.mu.Unlock()
	if windows != 1 {
		t.Fatal("parallel opens created multiple windows")
	}
	if err := m.Stop("example.ui"); err != nil {
		t.Fatal(err)
	}
	if waitEvent(t, events, 3*time.Second).Failed {
		t.Fatal("shared guest failed")
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

func TestNativeHostFocusPreservesContent(t *testing.T) {
	if nativeGuest == "" {
		t.Skip("set SINAN_NATIVE_UI_TEST=1 on macOS")
	}
	m := installed(t, `
const field=document.querySelector('#input');
field.value='keep-input';
const send=body=>window.webkit.messageHandlers.sinan.postMessage(body);
send({kind:'ready',version:1});
setTimeout(()=>{if(field.value==='keep-input')send({kind:'close'});},1500);
`)
	session, err := m.OpenUICommand(context.Background(), "example.ui", "window")
	if err != nil {
		t.Fatal(err)
	}
	process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{Executable: nativeGuest, Lifetime: 8 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Stop()
	for range 2 {
		if err = process.Focus(); err != nil {
			t.Fatal(err)
		}
	}
	result := process.Wait()
	var report struct {
		FocusHandled        int  `json:"focusHandled"`
		CloseReceived       bool `json:"closeReceived"`
		EventLoopResponsive bool `json:"eventLoopResponsive"`
	}
	if result.Err != nil || json.Unmarshal(result.Report, &report) != nil || report.FocusHandled != 2 || !report.CloseReceived || !report.EventLoopResponsive {
		t.Fatal("native focus did not preserve the live document")
	}
}

func TestAttachedPanelDetachesFromMainVisibility(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "panel")
	if handled, err := c.Main(true); handled || err != nil {
		t.Fatal("empty main panel handled")
	}
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if handled, err := c.Main(true); !handled || err != nil {
			t.Fatal("attached panel toggle failed", err)
		}
	}
	c.mu.Lock()
	entry := c.panel
	c.mu.Unlock()
	pid := entry.process.PID()
	if err := entry.process.Detach(); err != nil {
		t.Fatal(err)
	}
	event := waitEvent(t, events, time.Second)
	if !event.Detached || event.ReturnToSearch {
		t.Fatal("detach unexpectedly reopened search")
	}
	if handled, err := c.Main(true); handled || err != nil {
		t.Fatal("detached window controlled by main shortcut")
	}
	if !entry.session.Active() || entry.process.PID() != pid {
		t.Fatal("detach replaced or revoked session")
	}
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	if handled, _ := c.Main(false); handled {
		t.Fatal("focusing detached window attached it again")
	}
}

func TestNativeHostPanelDetachPreservesDocument(t *testing.T) {
	if nativeGuest == "" {
		t.Skip("set SINAN_NATIVE_UI_TEST=1 on macOS")
	}
	m := installed(t, `
const field=document.querySelector('#input');
field.value='before-detach';
const send=body=>window.webkit.messageHandlers.sinan.postMessage(body);
send({kind:'ready',version:1});
setTimeout(()=>{if(field.value==='before-detach')send({kind:'close'});},2500);
`)
	session, err := m.OpenUICommand(context.Background(), "example.ui", "window")
	if err != nil {
		t.Fatal(err)
	}
	actions := make(chan string, 1)
	process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{
		Executable: nativeGuest, Panel: true, Lifetime: 8 * time.Second,
		WindowAction: func(action string) bool { actions <- action; return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Stop()
	for range 2 {
		if err := process.Toggle(); err != nil {
			t.Fatal(err)
		}
	}
	if err := process.Detach(); err != nil {
		t.Fatal(err)
	}
	if action := <-actions; action != "detach" {
		t.Fatal("wrong native action")
	}
	if err := process.Focus(); err != nil {
		t.Fatal(err)
	}
	result := process.Wait()
	var report struct {
		Detached            bool `json:"detached"`
		TogglesHandled      int  `json:"togglesHandled"`
		CloseReceived       bool `json:"closeReceived"`
		EventLoopResponsive bool `json:"eventLoopResponsive"`
	}
	if result.Err != nil || json.Unmarshal(result.Report, &report) != nil || !report.Detached || report.TogglesHandled != 2 || !report.CloseReceived || !report.EventLoopResponsive {
		t.Fatal("native panel did not retain document through detach", result.Err)
	}
}

func TestAttachedPanelBackReturnsToSearch(t *testing.T) {
	m := installed(t, "")
	c, events := controller(t, m, "back")
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	event := waitEvent(t, events, 2*time.Second)
	if !event.ReturnToSearch || event.Detached {
		t.Fatal("back did not restore search")
	}
	if handled, _ := c.Main(true); handled {
		t.Fatal("back left main panel attached")
	}
}

func installNextView(t *testing.T, m *kernel.Manager, missing bool) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "ui"), 0700); err != nil {
		t.Fatal(err)
	}
	if !missing {
		if err := os.WriteFile(filepath.Join(root, "ui", "index.html"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mf := manifest.Manifest{ID: "example.next", Name: "Next", Version: "0.1.0", APIVersion: "1", Runtime: manifest.Runtime{Kind: "declarative"}, Contributes: manifest.Contributions{Commands: []manifest.Command{{ID: "open", Title: "Next", UIView: "main"}}, UIViews: []protocol.UIView{{ID: "main", Entry: "index.html", BridgeVersion: "1"}}}}
	data, _ := json.Marshal(mf)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(root, false); err != nil {
		t.Fatal(err)
	}
}
func TestFailedPanelReplacementKeepsPreviousSession(t *testing.T) {
	for _, phase := range []string{"resource", "startup"} {
		t.Run(phase, func(t *testing.T) {
			m := installed(t, "")
			installNextView(t, m, phase == "resource")
			c, events := controller(t, m, "wait-revoke")
			if err := c.Open("example.ui", "main"); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			previous := c.panel
			if phase == "startup" {
				c.options.Args = []string{"silent"}
				c.options.StartupTimeout = 150 * time.Millisecond
			}
			c.mu.Unlock()
			if err := c.Open("example.next", "main"); err == nil {
				t.Fatal("broken candidate opened")
			}
			c.mu.Lock()
			same := c.panel == previous
			c.mu.Unlock()
			if !same || !previous.session.Active() {
				t.Fatal("failed candidate retired previous page")
			}
			if handled, err := c.Main(false); !handled || err != nil {
				t.Fatal("previous page no longer accepts focus", err)
			}
			select {
			case event := <-events:
				t.Fatalf("failed replacement closed previous guest: %s", event.PluginID)
			default:
			}
		})
	}
}
func TestSuccessfulPanelReplacementRetiresOnlyPreviousAttachedPage(t *testing.T) {
	m := installed(t, "")
	installNextView(t, m, false)
	c, events := controller(t, m, "wait-revoke")
	if err := c.Open("example.ui", "main"); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	previous := c.panel
	c.mu.Unlock()
	if err := c.Open("example.next", "main"); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	current := c.panel
	c.mu.Unlock()
	if current == previous || previous.session.Active() || !current.session.Active() {
		t.Fatal("replacement did not commit")
	}
	event := waitEvent(t, events, 3*time.Second)
	if event.PluginID != "example.ui" || event.ReturnToSearch || event.Failed {
		t.Fatal("old page exit disturbed committed page")
	}
}

func TestNativeHostJSONPluginCallsAndDraftRestart(t *testing.T) {
	if nativeGuest == "" {
		t.Skip("set SINAN_NATIVE_UI_TEST=1 on macOS")
	}
	source := t.TempDir()
	if err := os.CopyFS(source, os.DirFS("../../../plugins/json-tool")); err != nil {
		t.Fatal(err)
	}
	bridge, err := os.ReadFile("../../../packages/plugin-sdk/ui/index.js")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "ui", "bridge.js"), bridge, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(source, "backend"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = exec.Command("go", "build", "-o", filepath.Join(source, "backend", "plugin"), "../../../plugins/json-tool/cmd").Run(); err != nil {
		t.Fatal("build JSON backend", err)
	}
	entry, err := os.ReadFile(filepath.Join(source, "ui", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	entry = append(entry, []byte(`<script type="module" src="acceptance.js"></script>`)...)
	if err = os.WriteFile(filepath.Join(source, "ui", "index.html"), entry, 0600); err != nil {
		t.Fatal(err)
	}
	script := `
const call=request=>window.webkit.messageHandlers.sinanCall.postMessage(request);
let phase="load";
try {
const wait=async predicate=>{const end=Date.now()+4000;while(!predicate()){if(Date.now()>end)throw Error('test condition');await new Promise(r=>setTimeout(r,20));}};
const field=document.querySelector('#editor'),status=document.querySelector('#status');
const compact='{"n":900719925474099312345,"a":[2,1],"markup":"'+'<>&'.repeat(10000)+'"}';
await wait(()=>!field.disabled);
if(window._wails||window.__WAILS__)throw Error('full bridge exposed');
for(const request of [{method:'storage.get',pluginId:'system.settings'},{method:'command.execute',commandId:'open',input:''}]){
 let rejected=false;try{await call(request);}catch{rejected=true;}if(!rejected)throw Error('authority escaped');
}
if(field.value){if(field.value!==compact)throw Error('draft changed');}
else{
 field.value=compact;field.dispatchEvent(new Event('input'));
 phase='format';
 document.querySelector('#format').click();
 await wait(()=>field.value.includes('\n')&&status.textContent==='草稿已保存');
 if(!field.value.includes('900719925474099312345'))throw Error('number precision lost');
 // Native reparenting intentionally has no page-visible resize/event contract.
 // Let the Go side detach after the first durable save, then exercise RPC again.
 await new Promise(resolve=>setTimeout(resolve,800));
 if(!field.value.includes('900719925474099312345'))throw Error('detach lost input');
 phase='compact after detach';
 document.querySelector('#compact').click();
 await wait(()=>field.value===compact&&status.textContent==='草稿已保存');
 phase='invalid input';
 field.value='{oops';field.dispatchEvent(new Event('input'));document.querySelector('#format').click();
 await wait(()=>!document.querySelector('#error').hidden);
 if(field.value!=='{oops')throw Error('invalid input lost');
 field.value=compact;field.dispatchEvent(new Event('input'));
 await wait(()=>status.textContent==='草稿已保存');
}
} catch(reason) {
 const doc=await call({method:'storage.get'});
 await call({method:'storage.put',revision:doc.revision,value:{testFailure:phase,testError:String(reason)}});
}
window.webkit.messageHandlers.sinan.postMessage({kind:'close'});
`
	if err = os.WriteFile(filepath.Join(source, "ui", "acceptance.js"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for pass := 0; pass < 2; pass++ {
		m, err := kernel.New(root)
		if err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			if _, err = m.Install(source, true); err != nil {
				m.Close()
				t.Fatal(err)
			}
		}
		session, err := m.OpenUICommand(context.Background(), "tools.json", "open")
		if err != nil {
			m.Close()
			t.Fatal(err)
		}
		process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{Executable: nativeGuest, Lifetime: 10 * time.Second, Panel: true, WindowAction: func(action string) bool { return action == "detach" }})
		if err != nil {
			m.Close()
			t.Fatal(err)
		}
		if pass == 0 {
			until := time.Now().Add(4 * time.Second)
			draft := filepath.Join(root, ".sinan-state", "data", "tools.json.json")
			for time.Now().Before(until) {
				if _, err := os.Stat(draft); err == nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err = process.Detach(); err != nil {
				process.Stop()
				m.Close()
				t.Fatal("detach JSON editor", err)
			}
		}
		result := process.Wait()
		docBytes, _ := os.ReadFile(filepath.Join(root, ".sinan-state", "data", "tools.json.json"))
		var diagnostic struct {
			Value struct {
				TestFailure string
				TestError   string
			}
		}
		_ = json.Unmarshal(docBytes, &diagnostic)
		if diagnostic.Value.TestFailure != "" {
			m.Close()
			t.Fatalf("JSON test phase %s: %s", diagnostic.Value.TestFailure, diagnostic.Value.TestError)
		}
		m.Close()
		var report struct {
			Detached      bool
			CloseReceived bool
			Failure       string
		}
		if result.Err != nil || json.Unmarshal(result.Report, &report) != nil || !report.CloseReceived || report.Failure != "" || (pass == 0 && !report.Detached) {
			t.Fatalf("JSON UI pass %d failed: %v", pass, result.Err)
		}
	}
}
