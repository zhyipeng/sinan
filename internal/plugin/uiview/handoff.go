package uiview

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// Structural classification only; session and kernel still authorize every frame.
func IsImageDataFrame(raw []byte) bool {
	if len(raw) > MaxCallBytes {
		return false
	}
	fields, err := callObject(raw)
	if err != nil || len(fields) != 2 {
		return false
	}
	var method, input string
	if json.Unmarshal(fields["method"], &method) != nil || method != "image.upload.chunk" || json.Unmarshal(fields["input"], &input) != nil {
		return false
	}
	parts := strings.SplitN(input, ":", 3)
	if len(parts) != 3 || len(parts[0]) != 32 {
		return false
	}
	if _, e := hex.DecodeString(parts[0]); e != nil {
		return false
	}
	n, e := strconv.Atoi(parts[1])
	return e == nil && n >= 0 && len(parts[2]) > 0 && len(parts[2]) <= base64.StdEncoding.EncodedLen(36<<10)
}
func (s *Session) Suspend() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activation == nil {
		s.activation = make(chan struct{})
	}
}
func (s *Session) Activate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activation != nil {
		close(s.activation)
		s.activation = nil
	}
}
func (s *Session) WaitActivation(ctx context.Context) error {
	s.mu.Lock()
	gate := s.activation
	s.mu.Unlock()
	if gate == nil {
		return nil
	}
	select {
	case <-gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errors.New("目标页面准备已撤销")
	}
}
