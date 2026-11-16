package uiprocess

import (
	"encoding/json"
	"errors"
	"io"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"sinan/internal/plugin/uiview"
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
)

// Client is the trusted renderer's proxy. Authority and resource bytes stay in
// the parent; a revoked local gate never falls back to a resource cache.
type Control struct {
	ID   uint64
	Kind string
}

type Client struct {
	theme            *protocol.ThemeState
	detachShortcut   string
	focusRequests    chan Control
	init             Init
	reader           io.ReadCloser
	writer           io.WriteCloser
	callMu           sync.Mutex
	mu               sync.Mutex
	sequence         uint64
	pending          chan response
	expected         uint64
	closed           chan struct{}
	closeOnce        sync.Once
	revoked          bool
	gate             chan struct{}
	listeners        map[uint64]func()
	listenerSequence uint64
}

func Connect(reader io.ReadCloser, writer io.WriteCloser) (*Client, error) {
	scanner := newScanner(reader)
	if !scanner.Scan() {
		return nil, errors.New("UI 初始化管道已关闭")
	}
	var init Init
	if decode(scanner.Bytes(), &init) != nil || (init.BridgeVersion != "" && !protocol.SupportedUIBridge(init.BridgeVersion)) || (init.Theme != nil && theme.ValidateState(*init.Theme) != nil) || init.Kind != "init" || init.Version != 1 || init.Entry == "" || init.CSP != uiview.CSP || len(init.DetachShortcut) > 128 {
		return nil, errors.New("UI 初始化消息无效")
	}
	c := &Client{theme: init.Theme, focusRequests: make(chan Control, 1), init: init, detachShortcut: init.DetachShortcut, reader: reader, writer: writer, closed: make(chan struct{}), gate: make(chan struct{}), listeners: map[uint64]func(){}}
	go func() {
		var focusSequence uint64
		defer c.Close()
		for scanner.Scan() {
			var frame response
			if decode(scanner.Bytes(), &frame) != nil {
				return
			}
			if (len(frame.Result) > 0 || frame.Error != "") && frame.Kind != "reply" {
				return
			}
			if frame.Kind == "theme" {
				if frame.Theme == nil || theme.ValidateState(*frame.Theme) != nil || frame.ID != 0 || frame.OK || frame.Action != 0 || frame.Resource != nil || frame.DetachShortcut != "" || frame.Error != "" || len(frame.Result) > 0 {
					return
				}
				c.mu.Lock()
				if c.theme == nil || frame.Theme.Revision >= c.theme.Revision {
					c.theme = frame.Theme
				}
				c.mu.Unlock()
				continue
			}
			if frame.Theme != nil {
				return
			}
			if frame.Kind == "detach-shortcut" {
				if frame.ID != 0 || frame.OK || frame.Resource != nil || frame.Action != 0 || len(frame.DetachShortcut) == 0 || len(frame.DetachShortcut) > 128 {
					return
				}
				c.mu.Lock()
				c.detachShortcut = frame.DetachShortcut
				c.mu.Unlock()
				continue
			}
			if frame.DetachShortcut != "" {
				return
			}
			if frame.Kind == "revoke" && frame.ID == 0 && !frame.OK && frame.Resource == nil && frame.Action == 0 {
				c.revokeLocal()
				continue
			}
			if (frame.Kind == "focus" || frame.Kind == "toggle" || frame.Kind == "detach") && frame.ID > 0 && frame.ID == focusSequence+1 && !frame.OK && frame.Resource == nil && frame.Action == 0 {
				focusSequence = frame.ID
				select {
				case c.focusRequests <- Control{ID: frame.ID, Kind: frame.Kind}:
				default:
					return
				}
				continue
			}
			if frame.Kind != "reply" || frame.ID == 0 {
				return
			}
			c.mu.Lock()
			ch, id := c.pending, c.expected
			c.mu.Unlock()
			if ch == nil || id != frame.ID {
				return
			}
			select {
			case ch <- frame:
			default:
				return
			}
		}
	}()
	if _, err := c.call(request{Kind: "hello", Version: 1}, true); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
func (c *Client) Init() Init   { return c.init }
func (c *Client) Active() bool { c.mu.Lock(); defer c.mu.Unlock(); return !c.revoked }
func (c *Client) OnRevoke(signal func()) func() {
	c.mu.Lock()
	if c.revoked {
		c.mu.Unlock()
		signal()
		return func() {}
	}
	c.listenerSequence++
	id := c.listenerSequence
	c.listeners[id] = signal
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.listeners, id); c.mu.Unlock() }
}
func (c *Client) revokeLocal() {
	c.mu.Lock()
	if c.revoked {
		c.mu.Unlock()
		return
	}
	c.revoked = true
	close(c.gate)
	callbacks := make([]func(), 0, len(c.listeners))
	for _, signal := range c.listeners {
		callbacks = append(callbacks, signal)
	}
	c.listeners = nil
	c.mu.Unlock()
	for _, signal := range callbacks {
		signal()
	}
}
func (c *Client) Close() {
	c.closeOnce.Do(func() { c.revokeLocal(); close(c.closed); _ = c.reader.Close(); _ = c.writer.Close() })
}
func (c *Client) call(req request, allowRevoked bool) (response, error) {
	c.callMu.Lock()
	defer c.callMu.Unlock()
	c.mu.Lock()
	if c.revoked && !allowRevoked {
		c.mu.Unlock()
		return response{}, errors.New("UI 会话已撤销")
	}
	c.sequence++
	req.ID = c.sequence
	ch := make(chan response, 1)
	c.pending = ch
	c.expected = req.ID
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.pending = nil; c.mu.Unlock() }()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	written := make(chan error, 1)
	go func() { written <- writeFrame(c.writer, req) }()
	select {
	case err := <-written:
		if err != nil {
			c.Close()
			return response{}, errors.New("UI 管道写入失败")
		}
	case <-c.closed:
		return response{}, errors.New("UI 管道已关闭")
	case <-timer.C:
		c.Close()
		return response{}, errors.New("UI 管道超时")
	}
	gate := c.gate
	if allowRevoked {
		gate = nil
	}
	var frame response
	select {
	case frame = <-ch:
	case <-gate:
		// Close acknowledgements can precede revoke in the same pipe read batch.
		select {
		case frame = <-ch:
		default:
			return response{}, errors.New("UI 会话已撤销")
		}
	case <-c.closed:
		select {
		case frame = <-ch:
		default:
			return response{}, errors.New("UI 管道已关闭")
		}
	case <-timer.C:
		c.Close()
		return response{}, errors.New("UI 管道超时")
	}
	if !frame.OK {
		return frame, errors.New("UI 请求被拒绝")
	}
	return frame, nil
}
func (c *Client) Resource(url string) (uiview.Resource, bool) {
	if len(url) > 1024 {
		return uiview.Resource{}, false
	}
	frame, err := c.call(request{Kind: "resource", URL: url}, false)
	limit := uiview.MaxFileBytes
	entry, parseErr := neturl.Parse(c.init.Entry)
	if parseErr == nil && frame.Resource != nil && frame.Resource.MIME == "image/png" && strings.HasPrefix(url, "sinan-plugin://"+entry.Host+"/__sinan_input/") {
		limit = 4 << 20
	}
	if err != nil || frame.Resource == nil || len(frame.Resource.Data) > limit {
		return uiview.Resource{}, false
	}
	return *frame.Resource, true
}
func (c *Client) Message(data []byte) (uiview.Action, error) {
	if len(data) > uiview.MaxMessageBytes {
		return uiview.Rejected, errors.New("UI 消息超过限制")
	}
	frame, err := c.call(request{Kind: "message", Message: data}, false)
	if err != nil {
		return uiview.Rejected, err
	}
	if frame.Action == uiview.Close {
		c.revokeLocal()
	}
	if frame.Action != uiview.Ready && frame.Action != uiview.Close {
		c.Close()
		return uiview.Rejected, errors.New("UI 动作响应无效")
	}
	return frame.Action, nil
}
func (c *Client) Revoke() { c.revokeLocal(); _, _ = c.call(request{Kind: "revoke"}, true) }
func (c *Client) ProbeDisable() error {
	_, err := c.call(request{Kind: "probe-disable"}, false)
	if err == nil {
		c.revokeLocal()
	}
	return err
}
func (c *Client) Finish(report json.RawMessage) error {
	if len(report) > 4096 || !json.Valid(report) {
		return errors.New("UI 验证报告无效")
	}
	_, err := c.call(request{Kind: "finish", Report: report}, true)
	return err
}

// These controls belong to the trusted renderer, never to page messages.
func (c *Client) Controls() <-chan Control { return c.focusRequests }
func (c *Client) NextControl() Control {
	select {
	case control := <-c.focusRequests:
		return control
	default:
		return Control{}
	}
}
func (c *Client) WindowAction(action string) error {
	_, err := c.call(request{Kind: "window-action", WindowAction: action}, false)
	return err
}
func (c *Client) Focused(id uint64) error {
	_, err := c.call(request{Kind: "focused", Control: id}, false)
	return err
}

func (c *Client) DetachShortcut() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.detachShortcut == "" {
		return "Shift+Cmd+D"
	}
	return c.detachShortcut
}

func (c *Client) Call(data []byte) (json.RawMessage, error) {
	if len(data) > uiview.MaxCallBytes {
		return nil, errors.New("UI 请求超过限制")
	}
	frame, err := c.call(request{Kind: "call", Message: data}, false)
	if err != nil {
		return nil, err
	}
	if frame.Error != "" {
		return nil, errors.New(frame.Error)
	}
	if len(frame.Result) > uiview.MaxCallResultBytes || !json.Valid(frame.Result) {
		return nil, errors.New("UI 响应无效")
	}
	return frame.Result, nil
}

func (c *Client) ThemeState() *protocol.ThemeState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.theme == nil {
		return nil
	}
	copy := theme.CloneState(*c.theme)
	return &copy
}

func (c *Client) ThemeRevision() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.theme == nil {
		return 0
	}
	return c.theme.Revision
}
