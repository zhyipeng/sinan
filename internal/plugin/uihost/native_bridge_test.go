package uihost

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func bridgeFixture(t *testing.T) (*nativeBridge, *uiview.Session) {
	t.Helper()
	bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("<p>content</p>")}, "app.js": {Data: []byte("void 0")}}, "example.bridge", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	session, err := uiview.NewSession(uiview.Binding{PluginID: "example.bridge", ViewID: "main", BridgeVersion: "2", Fingerprint: strings.Repeat("ab", 32)}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := newNativeBridge(session, session.EntryURL())
	if err != nil {
		t.Fatal(err)
	}
	return bridge, session
}

func TestNativeBridgeRestrictsNavigationAndProvenance(t *testing.T) {
	b, session := bridgeFixture(t)
	for _, target := range []string{"https://example.com", "about:blank", b.entry + "#fragment", strings.Replace(b.entry, "example.bridge", "example.other", 1)} {
		if b.Navigate(target, true) {
			t.Fatal("non-entry navigation allowed")
		}
	}
	if b.Navigate(b.entry, false) {
		t.Fatal("subframe navigation allowed")
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			if b.Navigate(b.entry, true) {
				accepted.Add(1)
			}
		})
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatal("initial navigation was not single-use")
	}
	ready := []byte(`{"kind":"ready","version":2}`)
	if _, err := b.Message(b.entry, false, ready); err == nil {
		t.Fatal("subframe message accepted")
	}
	if _, err := b.Message("about:blank", true, ready); err == nil {
		t.Fatal("foreign message accepted")
	}
	if action, err := b.Message(b.entry, true, ready); err != nil || action != uiview.Ready {
		t.Fatal("valid handshake rejected", err)
	}
	session.Revoke()
	if b.Navigate(b.entry, true) {
		t.Fatal("revoked navigation accepted")
	}
	if _, ok := b.Resource("GET", b.entry); ok {
		t.Fatal("revoked resource returned")
	}
	if _, err := b.Message(b.entry, true, []byte(`{"kind":"close"}`)); err == nil {
		t.Fatal("revoked message accepted")
	}
}

func TestNativeBridgeRejectsResourceMethodsAndForeignURLs(t *testing.T) {
	b, _ := bridgeFixture(t)
	for _, method := range []string{"POST", "HEAD", "PUT", "get", ""} {
		if _, ok := b.Resource(method, b.entry); ok {
			t.Fatal("unsupported resource method accepted")
		}
	}
	for _, target := range []string{"https://example.com/image.png", "file:///etc/passwd", b.entry + "?query", b.entry + "#fragment", strings.Replace(b.entry, "example.bridge", "example.other", 1)} {
		if _, ok := b.Resource("GET", target); ok {
			t.Fatal("foreign/modified URL accepted")
		}
	}
	if _, ok := b.Resource("GET", b.entry); !ok {
		t.Fatal("own resource rejected")
	}
}

func TestNativeBridgeCallsKeepSessionBindingAndRevokeLateResults(t *testing.T) {
	b, session := bridgeFixture(t)
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	session.ConfigureCaller(func(ctx context.Context, req protocol.UIRequest) (json.RawMessage, error) {
		calls.Add(1)
		close(entered)
		<-release
		return json.RawMessage(`{"ok":true}`), nil
	})
	_, _ = b.Message(b.entry, true, []byte(`{"kind":"ready","version":2}`))
	request := []byte(`{"method":"storage.get"}`)
	for _, source := range []struct {
		url  string
		main bool
	}{{b.entry, false}, {"about:blank", true}, {b.entry + "#fragment", true}} {
		if _, err := b.Call(source.url, source.main, request); err == nil {
			t.Fatal("invalid caller accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid caller reached backend")
	}
	finished := make(chan error, 1)
	go func() { _, err := b.Call(b.entry, true, request); finished <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("backend call did not start")
	}
	session.Revoke()
	unblock()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("late result survived revocation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revoked call did not finish")
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected backend calls")
	}
}

func TestNativeBridgeInvalidTrafficConsumesBudget(t *testing.T) {
	b, session := bridgeFixture(t)
	for range 256 {
		if _, ok := b.Resource("POST", b.entry); ok {
			t.Fatal("POST accepted")
		}
	}
	if !session.Active() {
		t.Fatal("budget revoked too early")
	}
	_, _ = b.Resource("POST", b.entry)
	if session.Active() {
		t.Fatal("invalid request flood bypassed budget")
	}
}

func TestNativeBridgeImageFramesKeepIndependentBudget(t *testing.T) {
	bridge, s := bridgeFixture(t)
	s.Message([]byte(`{"kind":"ready","version":2}`))
	s.ConfigureCaller(func(context.Context, protocol.UIRequest) (json.RawMessage, error) {
		return json.RawMessage(`null`), nil
	})
	frame := []byte(`{"method":"image.upload.chunk","input":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:0:YQ=="}`)
	for range 228 {
		if _, err := bridge.Call(bridge.entry, true, frame); err != nil {
			t.Fatal(err)
		}
	}
	for range 120 {
		if _, err := bridge.Call(bridge.entry, true, []byte(`{"method":"dependency.status"}`)); err != nil {
			t.Fatal("image frames exhausted control calls", err)
		}
	}
	if _, err := bridge.Call(bridge.entry, true, frame); err == nil {
		t.Fatal("image frame quota ignored")
	}
	if !s.Active() {
		t.Fatal("ordinary native request budget consumed by data frames")
	}
}
