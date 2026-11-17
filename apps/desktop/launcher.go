package main

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/platform"
)

const (
	launcherWidth     = 800
	launcherMinHeight = 50
	launcherMaxHeight = 600
)

// launcher owns only native Shell behavior. Plugin business remains in the runtime.
type trustedWindow struct {
	window *application.WebviewWindow
	key    string
}

type launcher struct {
	inputTarget    uintptr
	window         *application.WebviewWindow
	mu             sync.Mutex
	shortcutError  string
	opacityPercent int
	screenID       string
	detached       map[uint]trustedWindow
	newWindow      func() *application.WebviewWindow
	dialogs        map[*application.WebviewWindow]int
}

func (l *launcher) show() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.showLocked()
}

func (l *launcher) toggle() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dialogs[l.window] > 0 {
		return
	}
	if l.window.IsVisible() {
		l.window.Hide()
		return
	}
	l.showLocked()
}

func (l *launcher) showLocked() {
	if l.dialogs[l.window] > 0 {
		return
	}
	application.InvokeSync(func() {
		if target := platform.CaptureInputTarget(); target != 0 {
			l.inputTarget = target
		}
	})
	l.window.SetSize(launcherWidth, launcherMinHeight)
	var id string
	var ok bool
	application.InvokeSync(func() { id, ok = platform.CursorScreenID() })
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

// Native directory dialogs temporarily own focus without dismissing the panel.
func (l *launcher) beginDialog(window *application.WebviewWindow) func() {
	l.mu.Lock()
	if l.dialogs == nil {
		l.dialogs = make(map[*application.WebviewWindow]int)
	}
	l.dialogs[window]++
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.dialogs[window]--
		if l.dialogs[window] == 0 {
			delete(l.dialogs, window)
		}
	}
}

func (l *launcher) shouldHideOnBlur(window *application.WebviewWindow) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return window != nil && l.window == window && l.dialogs[window] == 0
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
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(appIcon)
	}
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
	restoreOpacity := l.opacityPercent >= 30 && l.opacityPercent < 100
	l.opacityPercent = 100
	l.mu.Unlock()
	window.RegisterKeyBinding("escape", nil)
	window.SetFrameless(false)
	window.SetTitle(title)
	window.SetAlwaysOnTop(false)
	window.SetResizable(true)
	x, y := window.Position()
	window.SetSize(launcherWidth, launcherMaxHeight)
	window.SetPosition(x, y)
	application.InvokeSync(func() {
		platform.DetachPanel(window.NativeWindow())
		// A detached view no longer carries the main window's transparency.
		if restoreOpacity {
			if err := platform.SetWindowOpacity(window.NativeWindow(), 100); err != nil {
				window.DispatchWailsEvent(&application.CustomEvent{Name: "window-opacity:error", Data: err.Error(), Sender: "kernel.settings"})
			}
		}
	})
	window.DispatchWailsEvent(&application.CustomEvent{Name: "view:detached", Sender: "kernel.views"})
	window.Show()
	window.Focus()
	return nil
}
