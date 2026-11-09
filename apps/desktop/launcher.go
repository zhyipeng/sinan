package main

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/platform"
)

const (
	launcherWidth     = 680
	launcherMinHeight = 50
	launcherMaxHeight = 560
)

// launcher owns only native Shell behavior. Plugin business remains in the runtime.
type trustedWindow struct {
	window *application.WebviewWindow
	key    string
}

type launcher struct {
	window        *application.WebviewWindow
	mu            sync.Mutex
	shortcutError string
	screenID      string
	detached      map[uint]trustedWindow
	newWindow     func() *application.WebviewWindow
}

func (l *launcher) show() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.showLocked()
}

func (l *launcher) toggle() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window.IsVisible() {
		l.window.Hide()
		return
	}
	l.showLocked()
}

func (l *launcher) showLocked() {
	l.window.SetSize(launcherWidth, launcherMinHeight)
	id, ok := platform.CursorScreenID()
	if !ok {
		if screen, _ := l.window.GetScreen(); screen != nil {
			id = screen.ID
		}
	}
	l.screenID = id
	if area, ok := screenWorkArea(id); ok {
		x, y := launcherPosition(area)
		l.window.SetPosition(x, y)
	} else {
		l.window.Center()
	}
	l.window.Show()
	l.window.Focus()
	// EmitEvent broadcasts through the application bus; Sender is not a target.
	l.window.DispatchWailsEvent(&application.CustomEvent{Name: "launcher:show", Sender: "kernel.launcher"})
}

func (l *launcher) resize(window *application.WebviewWindow, height int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if window == nil || l.window != window {
		return
	}
	height = max(launcherMinHeight, min(launcherMaxHeight, height))
	x, y := l.window.Position()
	if area, ok := screenWorkArea(l.screenID); ok {
		height = min(height, max(launcherMinHeight, area.Y+area.Height-y-16))
	}
	// macOS changes the origin when resizing; restore the top-left anchor.
	l.window.SetSize(launcherWidth, height)
	l.window.SetPosition(x, y)
}

func launcherPosition(area application.Rect) (int, int) {
	return area.X + (area.Width-launcherWidth)/2, area.Y + area.Height/5
}

func screenWorkArea(id string) (application.Rect, bool) {
	var native platform.WorkArea
	var ok bool
	application.InvokeSync(func() { native, ok = platform.ScreenWorkArea(id) })
	if ok {
		return application.Rect{X: native.X, Y: native.Y, Width: native.Width, Height: native.Height}, true
	}
	if screen := application.GetScreenByID(id); screen != nil {
		return screen.WorkArea, true
	}
	return application.Rect{}, false
}

func (l *launcher) setShortcutError(message string) {
	l.mu.Lock()
	l.shortcutError = message
	l.mu.Unlock()
}
func (l *launcher) status() string { l.mu.Lock(); defer l.mu.Unlock(); return l.shortcutError }

func configureTray(app *application.App, show func()) *application.SystemTray {
	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("司南")
	menu := app.NewMenu()
	menu.Add("显示司南").OnClick(func(_ *application.Context) { go show() })
	menu.AddSeparator()
	menu.Add("退出司南").OnClick(func(_ *application.Context) { app.Quit() })
	tray.SetMenu(menu)
	return tray
}

func (l *launcher) isMain(window application.Window) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.window != nil && l.window == window
}
func (l *launcher) forget(window application.Window) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.detached, window.ID())
}
func (l *launcher) caller(ctx context.Context) *application.WebviewWindow {
	window, _ := ctx.Value(application.WindowKey).(*application.WebviewWindow)
	if window == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window == window {
		return window
	}
	if entry, ok := l.detached[window.ID()]; ok && entry.window == window {
		return window
	}
	return nil
}
func (l *launcher) dispatch(event *application.CustomEvent) {
	l.mu.Lock()
	windows := []*application.WebviewWindow{l.window}
	for _, entry := range l.detached {
		windows = append(windows, entry.window)
	}
	l.mu.Unlock()
	for _, window := range windows {
		if window != nil {
			window.DispatchWailsEvent(event)
		}
	}
}
func (l *launcher) closePlugin(pluginID string) {
	l.mu.Lock()
	var windows []*application.WebviewWindow
	for _, entry := range l.detached {
		if strings.HasPrefix(entry.key, pluginID+"/") {
			windows = append(windows, entry.window)
		}
	}
	l.mu.Unlock()
	for _, window := range windows {
		window.Close()
	}
}
func (l *launcher) hide(window *application.WebviewWindow) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if window != nil && l.window == window {
		window.Hide()
	}
}
func (l *launcher) detach(window *application.WebviewWindow, title, key string) error {
	l.mu.Lock()
	if window == nil || window != l.window || l.newWindow == nil {
		l.mu.Unlock()
		return errors.New("当前界面无法拆出")
	}
	// Keep the existing WebView and React tree. Only the search role is replaced.
	replacement := l.newWindow()
	l.detached[window.ID()] = trustedWindow{window: window, key: key}
	l.window = replacement
	l.mu.Unlock()
	window.RegisterKeyBinding("escape", nil)
	window.SetFrameless(false)
	window.SetTitle(title)
	window.SetAlwaysOnTop(false)
	window.SetResizable(true)
	application.InvokeSync(func() { platform.DetachPanel(window.NativeWindow()) })
	window.DispatchWailsEvent(&application.CustomEvent{Name: "view:detached", Sender: "kernel.views"})
	window.Show()
	window.Focus()
	return nil
}
