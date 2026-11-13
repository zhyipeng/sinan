package uihost

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"

	"sinan/internal/plugin/uiview"
)

// nativeBridge is shared by native renderer adapters. Source and frame identity
// must come from the engine's event, never from fields in a page message.
// It intentionally exposes no Wails bindings or plugin-selection parameters.
type nativeBridge struct {
	session   Session
	entry     string
	mu        sync.Mutex
	navigated bool
	requests  int
	bytes     int
}

func newNativeBridge(session Session, entry string) (*nativeBridge, error) {
	u, err := url.Parse(entry)
	if session == nil || !session.Active() || len(entry) > 1024 || err != nil || u.Scheme != "sinan-plugin" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return nil, errors.New("原生插件会话入口无效")
	}
	if bound, ok := session.(interface{ EntryURL() string }); ok && bound.EntryURL() != entry {
		return nil, errors.New("原生插件会话入口不匹配")
	}
	return &nativeBridge{session: session, entry: entry}, nil
}

func (b *nativeBridge) consume(size int) bool {
	b.mu.Lock()
	b.requests++
	b.bytes += size
	allowed := b.requests <= 256 && b.bytes <= 8<<20
	b.mu.Unlock()
	if !allowed {
		b.session.Revoke()
	}
	return allowed && b.session.Active()
}

// Only the initial host-requested main document may navigate. Reloads,
// fragments, popups, foreign resources and subframes cannot reopen a session.
func (b *nativeBridge) Navigate(target string, mainFrame bool) bool {
	if !b.session.Active() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.navigated || !mainFrame || target != b.entry {
		return false
	}
	b.navigated = true
	return true
}

func (b *nativeBridge) Resource(method, target string) (uiview.Resource, bool) {
	if !b.consume(0) || method != "GET" || len(target) > 1024 {
		return uiview.Resource{}, false
	}
	resource, ok := b.session.Resource(target)
	b.mu.Lock()
	b.bytes += len(resource.Data)
	allowed := b.bytes <= 8<<20
	b.mu.Unlock()
	if !allowed {
		b.session.Revoke()
	}
	if !ok || !allowed || !b.session.Active() {
		return uiview.Resource{}, false
	}
	return resource, true
}

func (b *nativeBridge) Message(source string, mainFrame bool, data []byte) (uiview.Action, error) {
	if !b.consume(0) || !mainFrame || source != b.entry || len(data) > uiview.MaxMessageBytes {
		return uiview.Rejected, errors.New("原生插件消息来源或预算无效")
	}
	return b.session.Message(data)
}

func (b *nativeBridge) Call(source string, mainFrame bool, data []byte) (json.RawMessage, error) {
	if !b.consume(0) || !mainFrame || source != b.entry || len(data) > uiview.MaxCallBytes {
		return nil, errors.New("原生插件调用来源或预算无效")
	}
	var value json.RawMessage
	var err error
	switch caller := b.session.(type) {
	case interface {
		Call(context.Context, []byte) (json.RawMessage, error)
	}:
		value, err = caller.Call(context.Background(), data)
	case interface {
		Call([]byte) (json.RawMessage, error)
	}:
		value, err = caller.Call(data)
	default:
		return nil, errors.New("原生插件调用不可用")
	}
	if !b.session.Active() {
		return nil, errors.New("原生插件会话已关闭")
	}
	return value, err
}
