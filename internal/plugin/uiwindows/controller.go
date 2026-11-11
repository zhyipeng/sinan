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
	"sinan/internal/theme"
	protocol "sinan/packages/plugin-protocol"
)

type Event struct {
	PluginID       string `json:"pluginId"`
	ViewID         string `json:"viewId"`
	ReturnToSearch bool   `json:"returnToSearch"`
	Detached       bool   `json:"detached"`
	Failed         bool   `json:"failed"`
}

type windowEntry struct {
	attached bool
	session  *uiview.Session
	process  *uiprocess.Process
	ready    chan struct{}
	err      error // immutable after ready is closed
}

type Controller struct {
	preferenceRevision uint64
	manager            *kernel.Manager
	options            uiprocess.Options
	onExit             func(Event)
	ctx                context.Context
	cancel             context.CancelFunc
	mu                 sync.Mutex
	panel              *windowEntry
	closed             bool
	opening            map[string]*windowEntry
	active             sync.WaitGroup
}

func New(manager *kernel.Manager, executable string, onExit func(Event)) (*Controller, error) {
	info, err := os.Lstat(executable)
	if !filepath.IsAbs(executable) || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("UI 客体程序不可用，请重新构建宿主")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{manager: manager, options: uiprocess.Options{Executable: executable, Lifetime: time.Hour, Panel: true}, ctx: ctx, cancel: cancel, onExit: onExit, opening: map[string]*windowEntry{}}, nil
}

func (c *Controller) Open(pluginID, viewID string) error {
	return c.open(func(ctx context.Context) (uiview.Binding, error) {
		return c.manager.ResolveUIView(ctx, pluginID, viewID)
	}, func(ctx context.Context) (*uiview.Session, error) { return c.manager.OpenUIView(ctx, pluginID, viewID) })
}
func (c *Controller) OpenCommand(pluginID, commandID string) error {
	return c.open(func(ctx context.Context) (uiview.Binding, error) {
		return c.manager.ResolveUICommand(ctx, pluginID, commandID)
	}, func(ctx context.Context) (*uiview.Session, error) {
		return c.manager.OpenUICommand(ctx, pluginID, commandID)
	})
}

func (c *Controller) open(resolve func(context.Context) (uiview.Binding, error), prepare func(context.Context) (*uiview.Session, error)) (err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("UI 运行时已关闭")
	}
	c.active.Add(1)
	c.mu.Unlock()
	transferred := false
	var session *uiview.Session
	var owned *windowEntry
	var key string
	var previous *windowEntry
	candidateStarted := false
	defer func() {
		if !transferred {
			if session != nil {
				session.Revoke()
			}
			if owned != nil {
				c.mu.Lock()
				if c.opening[key] == owned {
					delete(c.opening, key)
				}
				if c.panel == owned {
					c.panel = nil
				}
				owned.err = err
				close(owned.ready)
				c.mu.Unlock()
			}
			if candidateStarted && previous != nil {
				c.mu.Lock()
				restore := !c.closed && c.panel == previous && previous.attached
				c.mu.Unlock()
				if restore && previous.session.Active() {
					_ = previous.process.Focus()
				}
			}
			c.active.Done()
		}
	}()
	binding, err := resolve(c.ctx)
	if err != nil {
		return err
	}
	pluginID, viewID := binding.PluginID, binding.ViewID
	key = pluginID + "/" + viewID
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("UI 运行时已关闭")
	}
	if existing := c.opening[key]; existing != nil {
		c.mu.Unlock()
		// Fresh metadata validates the installed declaration. It never replaces
		// or revives an old revoked instance, even when fingerprints match.
		waiting := false
		select {
		case <-existing.ready:
		default:
			waiting = true
			select {
			case <-existing.ready:
			case <-c.ctx.Done():
				return errors.New("UI 运行时已关闭")
			}
		}
		if existing.err != nil {
			return existing.err
		}
		if !existing.session.Active() || existing.session.Binding() != binding {
			return errors.New("窗口正在关闭，请稍后重试")
		}
		if waiting {
			return nil
		}
		return existing.process.Focus()
	}
	previous = c.panel
	owned = &windowEntry{ready: make(chan struct{}), attached: c.options.Panel}
	c.opening[key] = owned
	c.mu.Unlock()

	session, err = prepare(c.ctx)
	if err != nil {
		return err
	}
	if session.Binding() != binding {
		return errors.New("插件已变化，请重试")
	}
	owned.session = session
	c.mu.Lock()
	options := c.options
	c.mu.Unlock()
	options.WindowAction = func(action string) bool {
		c.mu.Lock()
		if c.closed || !owned.attached || c.panel != owned {
			c.mu.Unlock()
			return false
		}
		owned.attached = false
		if c.panel == owned {
			c.panel = nil
		}
		c.mu.Unlock()
		if c.onExit != nil {
			go c.onExit(Event{PluginID: pluginID, ViewID: viewID, Detached: action == "detach", ReturnToSearch: action == "back"})
		}
		return true
	}
	candidateStarted = true
	process, err := uiprocess.Start(c.ctx, session, options)
	if err != nil {
		return err
	}
	if c.ctx.Err() != nil {
		process.Stop()
		return errors.New("UI 运行时已关闭")
	}
	c.mu.Lock()
	owned.process = process
	// Never write a pipe under c.mu: window-action callbacks take this mutex.
	shortcut, revision := c.options.DetachShortcut, c.preferenceRevision
	currentTheme := c.options.Theme
	c.mu.Unlock()
	if currentTheme != nil {
		_ = process.SetTheme(*currentTheme)
	}
	if shortcut != "" {
		_ = process.SetDetachShortcut(shortcut, revision)
	}
	c.mu.Lock()
	var retired *windowEntry
	if owned.attached {
		retired = c.panel
		c.panel = owned
	}
	close(owned.ready)
	c.mu.Unlock()
	transferred = true
	if retired != nil && retired != owned {
		retired.session.Revoke()
	}
	go func() {
		result := process.Wait()
		var report *struct {
			Failure string `json:"failure"`
		}
		failed := result.Err != nil || json.Unmarshal(result.Report, &report) != nil || report == nil || report.Failure != ""
		c.mu.Lock()
		if c.opening[key] == owned {
			delete(c.opening, key)
		}
		returnToSearch := c.panel == owned
		if returnToSearch {
			c.panel = nil
		}
		closed := c.closed
		c.mu.Unlock()
		c.active.Done()
		if !closed && c.onExit != nil {
			c.onExit(Event{PluginID: pluginID, ViewID: viewID, Failed: failed, ReturnToSearch: returnToSearch})
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

// Main handles only the attached panel. Detached windows never participate.
func (c *Controller) Main(toggle bool) (bool, error) {
	c.mu.Lock()
	entry := c.panel
	c.mu.Unlock()
	if entry == nil {
		return false, nil
	}
	select {
	case <-entry.ready:
	case <-c.ctx.Done():
		return false, nil
	}
	c.mu.Lock()
	attached := c.panel == entry && entry.attached
	c.mu.Unlock()
	if !attached || entry.err != nil || entry.process == nil || !entry.session.Active() {
		return false, nil
	}
	if toggle {
		return true, entry.process.Toggle()
	}
	return true, entry.process.Focus()
}

func (c *Controller) SetDetachShortcut(value string) {
	if len(value) == 0 || len(value) > 128 {
		return
	}
	c.mu.Lock()
	c.preferenceRevision++
	revision := c.preferenceRevision
	c.options.DetachShortcut = value
	var processes []*uiprocess.Process
	for _, entry := range c.opening {
		if entry.process != nil {
			processes = append(processes, entry.process)
		}
	}
	c.mu.Unlock()
	for _, process := range processes {
		_ = process.SetDetachShortcut(value, revision)
	}
}

func (c *Controller) SetTheme(state protocol.ThemeState) {
	if theme.ValidateState(state) != nil {
		return
	}
	copy := theme.CloneState(state)
	c.mu.Lock()
	if c.options.Theme != nil && state.Revision < c.options.Theme.Revision {
		c.mu.Unlock()
		return
	}
	c.options.Theme = &copy
	var processes []*uiprocess.Process
	for _, entry := range c.opening {
		if entry.process != nil {
			processes = append(processes, entry.process)
		}
	}
	c.mu.Unlock()
	for _, process := range processes {
		_ = process.SetTheme(copy)
	}
}
