// Package uiwindows owns desktop guest lifetimes, independent of Shell focus.
package uiwindows

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sinan/internal/kernel"
	"sinan/internal/plugin/uiprocess"
	"sinan/internal/plugin/uiview"
)

type Event struct {
	PluginID string `json:"pluginId"`
	ViewID   string `json:"viewId"`
	Failed   bool   `json:"failed"`
}

type Controller struct {
	manager *kernel.Manager
	options uiprocess.Options
	onExit  func(Event)
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	opening map[string]bool
	active  sync.WaitGroup
}

func New(manager *kernel.Manager, executable string, onExit func(Event)) (*Controller, error) {
	info, err := os.Lstat(executable)
	if !filepath.IsAbs(executable) || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("UI 客体程序不可用，请重新构建宿主")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{manager: manager, options: uiprocess.Options{Executable: executable, Lifetime: time.Hour}, ctx: ctx, cancel: cancel, onExit: onExit, opening: map[string]bool{}}, nil
}

func (c *Controller) Open(pluginID, viewID string) error {
	return c.open(func(ctx context.Context) (*uiview.Session, error) { return c.manager.OpenUIView(ctx, pluginID, viewID) })
}
func (c *Controller) OpenCommand(pluginID, commandID string) error {
	return c.open(func(ctx context.Context) (*uiview.Session, error) {
		return c.manager.OpenUICommand(ctx, pluginID, commandID)
	})
}

func (c *Controller) open(prepare func(context.Context) (*uiview.Session, error)) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("UI 运行时已关闭")
	}
	c.active.Add(1)
	c.mu.Unlock()
	transferred, reserved := false, false
	var session *uiview.Session
	var key string
	defer func() {
		if !transferred {
			if session != nil {
				session.Revoke()
			}
			if reserved {
				c.mu.Lock()
				delete(c.opening, key)
				c.mu.Unlock()
			}
			c.active.Done()
		}
	}()
	var err error
	session, err = prepare(c.ctx)
	if err != nil {
		return err
	}
	binding := session.Binding()
	pluginID, viewID := binding.PluginID, binding.ViewID
	key = pluginID + "/" + viewID
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("UI 运行时已关闭")
	}
	if c.opening[key] {
		c.mu.Unlock()
		return errors.New("此视图已经打开")
	}
	c.opening[key] = true
	reserved = true
	c.mu.Unlock()
	process, err := uiprocess.Start(c.ctx, session, c.options)
	if err != nil {
		return err
	}
	if c.ctx.Err() != nil {
		process.Stop()
		return errors.New("UI 运行时已关闭")
	}
	transferred = true
	go func() {
		result := process.Wait()
		var report *struct {
			Failure string `json:"failure"`
		}
		failed := result.Err != nil || json.Unmarshal(result.Report, &report) != nil || report == nil || report.Failure != ""
		c.mu.Lock()
		delete(c.opening, key)
		closed := c.closed
		c.mu.Unlock()
		// Reaping must not wait for a UI callback that may need the main thread.
		c.active.Done()
		if !closed && c.onExit != nil {
			c.onExit(Event{PluginID: pluginID, ViewID: viewID, Failed: failed})
		}
	}()
	return nil
}

// Close cancels startups and running guests and waits until all have been reaped.
func (c *Controller) Close() {
	c.mu.Lock()
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.active.Wait()
}
