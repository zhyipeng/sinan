package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"sinan/internal/plugin/transport"
	protocol "sinan/packages/plugin-protocol"
)

type notificationSender func(context.Context, protocol.NotificationParams) error

// SetNotificationSender configures the optional desktop adapter before plugin activation.
func (m *Manager) SetNotificationSender(send func(context.Context, protocol.NotificationParams) error) {
	m.notificationSender = send
}

func notificationHandler(permissions []string, send notificationSender) transport.HostHandler {
	allowed := slices.Contains(permissions, "notifications.send")
	var mu sync.Mutex
	var window time.Time
	count := 0
	return func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		if method != "notifications.send" || !allowed || send == nil {
			return nil, errors.New("未声明通知权限或桌面通知不可用")
		}
		var params protocol.NotificationParams
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var extra any
		if len(raw) > 4096 || !utf8.Valid(raw) || decoder.Decode(&params) != nil || decoder.Decode(&extra) != io.EOF ||
			strings.TrimSpace(params.Title) == "" || len(params.Title) > protocol.NotificationMaxTitleBytes ||
			len(params.Body) > protocol.NotificationMaxBodyBytes || strings.ContainsFunc(params.Title, unicode.IsControl) ||
			strings.ContainsFunc(params.Body, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
			return nil, errors.New("通知标题或正文无效")
		}
		mu.Lock()
		if time.Since(window) >= time.Minute {
			window, count = time.Now(), 0
		}
		if count >= 24 {
			mu.Unlock()
			return nil, errors.New("通知发送过于频繁")
		}
		count++
		mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := send(ctx, params); err != nil {
			return nil, err
		}
		return struct{}{}, nil
	}
}
