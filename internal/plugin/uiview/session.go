package uiview

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

const MaxMessageBytes = 4096
const MaxSessionMessages = 32

type Binding struct {
	HeaderTitle   string
	HeaderSource  string
	Title         string
	ViewID        string
	BridgeVersion string
	PluginID      string
	Fingerprint   string
}
type Action int

const (
	Rejected Action = iota
	Ready
	Close
)

// Session identity is the host-held object, never a plugin ID supplied by a page.
// Binding is descriptive metadata, not a credential for reopening a session.
type Session struct {
	caller         func(context.Context, protocol.UIRequest) (json.RawMessage, error)
	calls          chan struct{}
	callWindow     time.Time
	callCount      int
	mu             sync.Mutex
	binding        Binding
	bundle         *Bundle
	launchImage    Resource
	launchImageURL string
	entry          string
	ready          bool
	revoked        bool
	messages       int
	done           chan struct{}
	signaled       chan struct{}
	listeners      map[uint64]func()
	sequence       uint64
}

func NewSession(binding Binding, bundle *Bundle) (*Session, error) {
	digest, err := hex.DecodeString(binding.Fingerprint)
	if !manifest.ValidViewID(binding.ViewID) || !protocol.SupportedUIBridge(binding.BridgeVersion) || bundle == nil || !manifest.ValidID(binding.PluginID) || binding.PluginID != bundle.owner || err != nil || len(digest) != 32 {
		return nil, errors.New("UI 会话绑定无效")
	}
	return &Session{binding: binding, bundle: bundle, entry: bundle.EntryURL(), done: make(chan struct{}), signaled: make(chan struct{}), listeners: map[uint64]func(){}}, nil
}
func (s *Session) Binding() Binding      { return s.binding }
func (s *Session) EntryURL() string      { return s.entry }
func (s *Session) Done() <-chan struct{} { return s.done }
func (s *Session) Active() bool          { s.mu.Lock(); defer s.mu.Unlock(); return !s.revoked }
func (s *Session) Resource(url string) (Resource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked {
		return Resource{}, false
	}
	if s.launchImageURL != "" && url == s.launchImageURL {
		return Resource{MIME: s.launchImage.MIME, Data: bytes.Clone(s.launchImage.Data)}, true
	}
	return s.bundle.Resource(url)
}

// Listeners must only signal cancellation and must not reenter Session or the
// kernel. Revoke runs them synchronously so native request gates close before
// the lifecycle operation returns. Removal does not wait for an in-flight call.
func (s *Session) OnRevoke(signal func()) func() {
	s.mu.Lock()
	if s.revoked {
		s.mu.Unlock()
		signal()
		return func() {}
	}
	s.sequence++
	id := s.sequence
	s.listeners[id] = signal
	s.mu.Unlock()
	return func() { s.mu.Lock(); delete(s.listeners, id); s.mu.Unlock() }
}
func (s *Session) revokeLocked() func() {
	if s.revoked {
		return func() { <-s.signaled }
	}
	s.revoked = true
	s.bundle = nil
	s.launchImage = Resource{}
	s.launchImageURL = ""
	close(s.done)
	listeners := make([]func(), 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.listeners = nil
	return func() {
		for _, signal := range listeners {
			signal()
		}
		close(s.signaled)
	}
}
func (s *Session) Revoke() {
	s.mu.Lock()
	signal := s.revokeLocked()
	s.mu.Unlock()
	signal()
}

func uniqueObject(data []byte) (map[string]json.RawMessage, error) { return parseObject(data, 2) }
func callObject(data []byte) (map[string]json.RawMessage, error)   { return parseObject(data, 5) }
func parseObject(data []byte, limit int) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid message")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil || len(fields) >= limit {
			return nil, errors.New("invalid fields")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("trailing content")
	}
	return fields, nil
}

// The initial narrow bridge only supports one ready handshake and close.
// Unknown operations, claimed identities and extra/duplicate fields are denied.
func (s *Session) Message(data []byte) (Action, error) {
	s.mu.Lock()
	if s.revoked {
		s.mu.Unlock()
		return Rejected, errors.New("UI 会话已撤销")
	}
	s.messages++
	if s.messages > MaxSessionMessages {
		signal := s.revokeLocked()
		s.mu.Unlock()
		signal()
		return Rejected, errors.New("UI 消息预算已耗尽")
	}
	reject := func() (Action, error) { s.mu.Unlock(); return Rejected, errors.New("UI 消息无效或不受支持") }
	if len(data) > MaxMessageBytes {
		return reject()
	}
	fields, err := uniqueObject(data)
	if err != nil {
		return reject()
	}
	var kind string
	if json.Unmarshal(fields["kind"], &kind) != nil {
		return reject()
	}
	if kind == "ready" && len(fields) == 2 && !s.ready {
		var version int
		if json.Unmarshal(fields["version"], &version) != nil || strconv.Itoa(version) != s.binding.BridgeVersion {
			return reject()
		}
		s.ready = true
		s.mu.Unlock()
		return Ready, nil
	}
	if kind == "close" && len(fields) == 1 && s.ready {
		signal := s.revokeLocked()
		s.mu.Unlock()
		signal()
		return Close, nil
	}
	return reject()
}

const MaxCallBytes = 64 << 10
const MaxCallResultBytes = 192 << 10

// ConfigureCaller is set once by the kernel, before publishing this session.
func (s *Session) ConfigureCaller(fn func(context.Context, protocol.UIRequest) (json.RawMessage, error)) {
	s.caller = fn
	s.calls = make(chan struct{}, 1)
}

func (s *Session) Call(ctx context.Context, data []byte) (json.RawMessage, error) {
	s.mu.Lock()
	if s.revoked || !s.ready || s.binding.BridgeVersion != protocol.UIRequestBridgeVersion || s.caller == nil {
		s.mu.Unlock()
		return nil, errors.New("UI 调用不可用")
	}
	if time.Since(s.callWindow) >= time.Minute {
		s.callWindow = time.Now()
		s.callCount = 0
	}
	s.callCount++
	if s.callCount > 120 {
		s.mu.Unlock()
		return nil, errors.New("UI 调用过于频繁，请稍后重试")
	}
	s.mu.Unlock()
	if len(data) > MaxCallBytes {
		return nil, errors.New("UI 请求超过 64 KiB")
	}
	// Validate exact, unique keys before typed decoding.
	fields, err := callObject(data)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{"method": true, "commandId": true, "input": true, "revision": true, "value": true}
	for key := range fields {
		if !allowed[key] || (key != "value" && bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null"))) {
			return nil, errors.New("UI 请求字段无效")
		}
	}
	var req protocol.UIRequest
	if json.Unmarshal(data, &req) != nil {
		return nil, errors.New("UI 请求无效")
	}
	switch req.Method {
	case "command.execute":
		if len(fields) > 3 || fields["revision"] != nil || fields["value"] != nil || req.CommandID == "" {
			return nil, errors.New("命令请求无效")
		}
	case "storage.get", "launch.get", "input.paste":
		if len(fields) != 1 {
			return nil, errors.New("数据读取请求无效")
		}
	case "storage.put":
		if len(fields) != 3 || fields["revision"] == nil || fields["value"] == nil {
			return nil, errors.New("数据保存请求无效")
		}
	default:
		return nil, errors.New("UI 方法不受支持")
	}
	select {
	case s.calls <- struct{}{}:
		defer func() { <-s.calls }()
	default:
		return nil, errors.New("已有 UI 调用正在处理")
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	unsubscribe := s.OnRevoke(cancel)
	defer unsubscribe()
	result, err := s.caller(ctx, req)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !s.Active() {
		return nil, errors.New("UI 调用已取消")
	}
	if len(result) > MaxCallResultBytes || !json.Valid(result) {
		return nil, errors.New("UI 返回值超过限制或无效")
	}
	return result, nil
}

// AttachLaunchImage is called only during kernel session construction. Resource
// URLs are scoped to the session and remain subject to normal revoke/budgets.
func (s *Session) AttachLaunchImage(data []byte, width, height int) protocol.UILaunchImage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revoked || s.ready || s.launchImageURL != "" {
		return protocol.UILaunchImage{}
	}
	s.launchImageURL = "sinan-plugin://" + s.binding.PluginID + "/__sinan_input/" + strings.ToLower(rand.Text()) + ".png"
	s.launchImage = Resource{MIME: "image/png", Data: bytes.Clone(data)}
	return protocol.UILaunchImage{URL: s.launchImageURL, MIMEType: "image/png", Width: width, Height: height, Bytes: len(data)}
}
