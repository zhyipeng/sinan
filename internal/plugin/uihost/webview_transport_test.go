package uihost

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"sinan/internal/plugin/uiview"
)

func TestWebviewTransportIsPerViewAndSessionBound(t *testing.T) {
	b, session := bridgeFixture(t)
	transport := newWebviewTransport(b)
	other := newWebviewTransport(b)
	if transport.origin == other.origin {
		t.Fatal("views share an origin")
	}
	if !strings.HasSuffix(transport.origin, ".sinan.invalid") {
		t.Fatal("unexpected virtual domain")
	}
	resource, ok := transport.resource("GET", transport.entry)
	if !ok || !bytes.Contains(resource.Data, []byte("content")) {
		t.Fatal("snapshot not returned")
	}
	for _, target := range []string{other.entry, b.entry, transport.entry + "?", transport.entry + "#", transport.entry + "?q=1", transport.entry + "#part", transport.origin + "/%69ndex.html", strings.Replace(transport.entry, "https:", "http:", 1), strings.Replace(transport.entry, "https://", "https://user@", 1), strings.Replace(transport.entry, ".invalid/", ".invalid:443/", 1), "file:///tmp/index.html", "https://example.com/index.html"} {
		if _, ok := transport.resource("GET", target); ok {
			t.Fatalf("unbound URL accepted: %s", target)
		}
		if transport.navigate(target, true) {
			t.Fatal("invalid navigation accepted")
		}
	}
	if _, ok := transport.resource("POST", transport.entry); ok {
		t.Fatal("non-GET accepted")
	}
	if transport.navigate(transport.entry, false) || !transport.navigate(transport.entry, true) || transport.navigate(transport.entry, true) {
		t.Fatal("initial top-level navigation not single-use")
	}
	if csp := transport.csp(); strings.Contains(csp, "sinan-plugin:") || !strings.Contains(csp, "script-src "+transport.origin) || !strings.Contains(csp, "connect-src 'none'") || !strings.Contains(csp, "frame-src 'none'") {
		t.Fatal("transport CSP lost restrictions")
	}
	session.Revoke()
	if _, ok := transport.resource("GET", transport.entry); ok {
		t.Fatal("revoked snapshot returned")
	}
}

func TestWebviewEnvelopeRequiresNativeSourceAndBoundedFields(t *testing.T) {
	b, session := bridgeFixture(t)
	transport := newWebviewTransport(b)
	ready := []byte(`{"channel":"sinan:request","kind":"message","id":1,"request":{"kind":"ready","version":2}}`)
	for _, source := range []string{b.entry, transport.entry + "#", newWebviewTransport(b).entry, "about:blank"} {
		if _, ok := transport.receive(source, ready); ok {
			t.Fatal("foreign native source accepted")
		}
	}
	invalid := []string{
		`{}`, `null`, string(ready) + ` {}`, strings.Replace(string(ready), `"id":1`, `"id":0`, 1), strings.Replace(string(ready), `"id":1`, `"id":9007199254740992`, 1), strings.Replace(string(ready), `"id":1`, `"id":1.2`, 1), strings.Replace(string(ready), `"id":1`, `"id":-1`, 1), strings.Replace(string(ready), `"message"`, `"detach"`, 1), strings.Replace(string(ready), `"channel"`, `"source"`, 1), strings.Replace(string(ready), `"request":`, `"pluginId":"other","request":`, 1), `{"channel":"sinan:request","kind":"call","id":1,"request":null}`,
	}
	for _, data := range invalid {
		if _, ok := transport.receive(transport.entry, []byte(data)); ok {
			t.Fatalf("invalid envelope accepted: %s", data)
		}
	}
	envelope, ok := transport.receive(transport.entry, ready)
	if !ok {
		t.Fatal("valid native source rejected")
	}
	if action, err := b.Message(b.entry, true, envelope.Request); err != nil || action != uiview.Ready {
		t.Fatal("handshake failed", err)
	}
	session.Revoke()
	if _, ok := transport.receive(transport.entry, ready); ok {
		t.Fatal("revoked message accepted")
	}
}

func TestWebviewEnvelopeBudgetsAndFloodRevocation(t *testing.T) {
	for _, kind := range []string{"message", "call"} {
		limit := uiview.MaxMessageBytes
		if kind == "call" {
			limit = uiview.MaxCallBytes
		}
		request := []byte(`"` + strings.Repeat("x", limit-2) + `"`)
		data, _ := json.Marshal(webviewEnvelope{webviewChannel, kind, 1, request})
		if _, err := decodeWebviewEnvelope(data); err != nil {
			t.Fatal("boundary request rejected", err)
		}
		request = append(request[:len(request)-1], 'x', '"')
		data, _ = json.Marshal(webviewEnvelope{webviewChannel, kind, 1, request})
		if _, err := decodeWebviewEnvelope(data); err == nil {
			t.Fatal("oversize request accepted")
		}
	}
	if _, err := decodeWebviewEnvelope(bytes.Repeat([]byte(" "), webviewMaxEnvelope+1)); err == nil {
		t.Fatal("oversize envelope accepted")
	}
	b, session := bridgeFixture(t)
	transport := newWebviewTransport(b)
	for range 257 {
		_, _ = transport.receive(transport.entry, []byte(`{}`))
	}
	if session.Active() {
		t.Fatal("invalid envelopes bypassed shared budget")
	}
}

func TestWebviewLaunchImageTranslationDoesNotRewriteUserData(t *testing.T) {
	b, _ := bridgeFixture(t)
	transport := newWebviewTransport(b)
	target := "sinan-plugin://example.bridge/__sinan_input/example.png"
	original, _ := json.Marshal(map[string]any{"commandId": "example", "input": target, "image": map[string]any{"url": target, "width": 128, "future": "preserved"}, "future": 42})
	value, err := transport.callResult([]byte(`{"method":"launch.get"}`), original)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Input string
		Image struct {
			URL    string
			Width  int
			Future string
		}
		Future int
	}
	if json.Unmarshal(value, &result) != nil || result.Input != target || result.Image.URL != transport.origin+"/__sinan_input/example.png" || result.Image.Width != 128 || result.Image.Future != "preserved" || result.Future != 42 {
		t.Fatal("translation changed unrelated data")
	}
	stored, err := transport.callResult([]byte(`{"method":"storage.get"}`), original)
	if err != nil || !bytes.Equal(stored, original) {
		t.Fatal("storage value rewritten")
	}
	for _, value := range []string{`null`, `{"image":null}`, `{"input":"unchanged"}`} {
		result, err := transport.callResult([]byte(`{"method":"launch.get"}`), []byte(value))
		if err != nil || string(result) != value {
			t.Fatal("imageless result changed")
		}
	}
	for _, url := range []string{"https://example.com/image.png", "sinan-plugin://other/__sinan_input/image.png", target + "?q=1", target + "#"} {
		data, _ := json.Marshal(map[string]any{"image": map[string]string{"url": url}})
		if _, err := transport.callResult([]byte(`{"method":"launch.get"}`), data); err == nil {
			t.Fatal("unbound launch image translated")
		}
	}
}
