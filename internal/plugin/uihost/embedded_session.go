package uihost

import (
	"context"
	"encoding/json"
	"errors"
	"sinan/internal/plugin/uiview"
	"sync"
)

// The direct native adapter preserves the old private pipe's aggregate budgets.
type embeddedSession struct {
	*uiview.Session
	mu            sync.Mutex
	requests      int
	resourceBytes int
}

func (s *embeddedSession) consume(bytes int) bool {
	s.mu.Lock()
	s.requests++
	s.resourceBytes += bytes
	allowed := s.requests <= 256 && s.resourceBytes <= 8<<20
	s.mu.Unlock()
	if !allowed {
		s.Session.Revoke()
	}
	return allowed
}
func (s *embeddedSession) Resource(url string) (uiview.Resource, bool) {
	resource, ok := s.Session.Resource(url)
	if !s.consume(len(resource.Data)) {
		return uiview.Resource{}, false
	}
	return resource, ok
}
func (s *embeddedSession) Message(data []byte) (uiview.Action, error) {
	if !s.consume(0) {
		return uiview.Rejected, errors.New("插件视图消息预算已耗尽")
	}
	return s.Session.Message(data)
}
func (s *embeddedSession) Call(data []byte) (json.RawMessage, error) {
	if !s.consume(0) {
		return nil, errors.New("插件视图调用预算已耗尽")
	}
	return s.Session.Call(context.Background(), data)
}
