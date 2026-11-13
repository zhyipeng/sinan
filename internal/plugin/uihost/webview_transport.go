package uihost

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"sinan/internal/plugin/uiview"
)

// webviewTransport gives WebView2 a fresh HTTPS origin without opening a socket
// or mapping a directory. The native adapter must answer *every* resource event,
// including rejected requests; leaving Response unset would permit networking.
type webviewTransport struct {
	bridge *nativeBridge
	origin string
	owner  string
	entry  string
}

func newWebviewTransport(b *nativeBridge) *webviewTransport {
	u, _ := url.Parse(b.entry)
	origin := "https://" + strings.ToLower(rand.Text()) + ".sinan.invalid"
	return &webviewTransport{bridge: b, origin: origin, owner: u.Host, entry: origin + u.Path}
}
func (t *webviewTransport) internal(target string) (string, bool) {
	if len(target) > 1024 {
		return "", false
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || "https://"+u.Host != t.origin || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Path == "" || target != t.origin+u.Path {
		return "", false
	}
	return "sinan-plugin://" + t.owner + u.Path, true
}
func (t *webviewTransport) public(target string) (string, bool) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "sinan-plugin" || u.Host != t.owner || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path == "" || target != "sinan-plugin://"+t.owner+u.Path {
		return "", false
	}
	return t.origin + u.Path, true
}
func (t *webviewTransport) resource(method, target string) (uiview.Resource, bool) {
	internal, ok := t.internal(target)
	if !ok {
		t.bridge.consume(0)
		return uiview.Resource{}, false
	}
	return t.bridge.Resource(method, internal)
}
func (t *webviewTransport) navigate(target string, main bool) bool {
	internal, ok := t.internal(target)
	return ok && t.bridge.Navigate(internal, main)
}
func (t *webviewTransport) csp() string {
	return strings.ReplaceAll(uiview.CSP, "sinan-plugin:", t.origin)
}

const webviewChannel = "sinan:request"
const webviewMaxEnvelope = uiview.MaxCallBytes + 1024

type webviewEnvelope struct {
	Channel string          `json:"channel"`
	Kind    string          `json:"kind"`
	ID      uint64          `json:"id"`
	Request json.RawMessage `json:"request"`
}

// IDs are JavaScript safe integers, not authority. Identity is supplied by the
// engine's top-level WebMessageReceived.Source, never by this envelope.
func decodeWebviewEnvelope(data []byte) (webviewEnvelope, error) {
	var e webviewEnvelope
	if len(data) > webviewMaxEnvelope {
		return e, errors.New("插件消息过大")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		return e, errors.New("插件消息格式无效")
	}
	if d.Decode(new(any)) != io.EOF || e.Channel != webviewChannel || e.ID == 0 || e.ID > 1<<53-1 || (e.Kind != "message" && e.Kind != "call") || len(e.Request) == 0 || bytes.Equal(e.Request, []byte("null")) {
		return e, errors.New("插件消息格式无效")
	}
	limit := uiview.MaxMessageBytes
	if e.Kind == "call" {
		limit = uiview.MaxCallBytes
	}
	if len(e.Request) > limit {
		return e, errors.New("插件消息过大")
	}
	return e, nil
}

// receive accepts provenance only from the native CoreWebView2 top-level
// event. Rejected envelopes also consume the shared session budget.
func (t *webviewTransport) receive(source string, data []byte) (webviewEnvelope, bool) {
	envelope, err := decodeWebviewEnvelope(data)
	if err != nil || source != t.entry || !t.bridge.session.Active() {
		t.bridge.consume(0)
		return webviewEnvelope{}, false
	}
	return envelope, true
}

// Only launch.get's image URL is a transport detail. Never rewrite user input,
// storage values, or arbitrary strings that happen to contain the private URL.
func (t *webviewTransport) callResult(request, value json.RawMessage) (json.RawMessage, error) {
	var call struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(request, &call) != nil || call.Method != "launch.get" {
		return value, nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	if len(result["image"]) == 0 || bytes.Equal(result["image"], []byte("null")) {
		return value, nil
	}
	var image map[string]json.RawMessage
	if err := json.Unmarshal(result["image"], &image); err != nil {
		return nil, err
	}
	var target string
	if json.Unmarshal(image["url"], &target) != nil {
		return nil, errors.New("插件图片地址无效")
	}
	public, ok := t.public(target)
	if !ok {
		return nil, errors.New("插件图片地址无效")
	}
	image["url"], _ = json.Marshal(public)
	result["image"], _ = json.Marshal(image)
	return json.Marshal(result)
}
