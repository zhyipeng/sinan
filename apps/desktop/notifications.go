package main

import (
	"context"
	"crypto/rand"
	"errors"
	"html"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	protocol "sinan/packages/plugin-protocol"
)

// Keep the Wails service private and make missing desktop support non-fatal.
type desktopNotifications struct {
	native   *notifications.NotificationService
	mu       sync.Mutex
	started  bool
	once     sync.Once
	authDone chan struct{}
	slot     chan struct{}
	granted  bool
}

func (n *desktopNotifications) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.native = notifications.New()
	n.authDone = make(chan struct{})
	n.slot = make(chan struct{}, 1)
	n.started = n.native.ServiceStartup(ctx, options) == nil
	return nil
}

func (n *desktopNotifications) ServiceShutdown() error {
	n.mu.Lock()
	started, native := n.started, n.native
	n.started = false
	n.mu.Unlock()
	if started {
		_ = native.ServiceShutdown()
	}
	return nil
}

func (n *desktopNotifications) send(ctx context.Context, value protocol.NotificationParams) error {
	n.mu.Lock()
	started, native := n.started, n.native
	n.mu.Unlock()
	if !started {
		return errors.New("系统通知服务不可用")
	}
	if runtime.GOOS == "darwin" {
		n.once.Do(func() {
			go func() {
				n.granted, _ = native.RequestNotificationAuthorization()
				close(n.authDone)
			}()
		})
		select {
		case <-n.authDone:
			if !n.granted {
				return errors.New("系统未授予通知权限")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	select {
	case n.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	n.mu.Lock()
	started = n.started
	n.mu.Unlock()
	if !started || ctx.Err() != nil {
		<-n.slot
		return errors.New("通知已取消")
	}
	body := value.Body
	if runtime.GOOS == "linux" {
		// Freedesktop notification bodies can interpret markup.
		body = html.EscapeString(body)
	}
	// Wails does not expose context cancellation for native sends. Hold one
	// slot until the call returns, without blocking desktop shutdown.
	sent := make(chan error, 1)
	go func() {
		defer func() { <-n.slot }()
		sent <- native.SendNotification(notifications.NotificationOptions{
			ID: rand.Text(), Title: value.Title, Body: body, Sound: &notifications.NotificationSound{Silent: true},
		})
	}()
	select {
	case err := <-sent:
		if err != nil {
			return errors.New("系统通知发送失败")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
