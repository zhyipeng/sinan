package uiview

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

const MaxMessageBytes = 4096
const MaxSessionMessages = 32

type Binding struct {
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
	mu        sync.Mutex
	binding   Binding
	bundle    *Bundle
	entry     string
	ready     bool
	revoked   bool
	messages  int
	done      chan struct{}
	signaled  chan struct{}
	listeners map[uint64]func()
	sequence  uint64
}

func NewSession(binding Binding, bundle *Bundle) (*Session, error) {
	digest, err := hex.DecodeString(binding.Fingerprint)
	if !manifest.ValidViewID(binding.ViewID) || binding.BridgeVersion != protocol.UIBridgeVersion || bundle == nil || !manifest.ValidID(binding.PluginID) || binding.PluginID != bundle.owner || err != nil || len(digest) != 32 {
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

func uniqueObject(data []byte) (map[string]json.RawMessage, error) {
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
		if !ok || fields[key] != nil || len(fields) >= 2 {
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
		if json.Unmarshal(fields["version"], &version) != nil || version != 1 {
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
