package main

import (
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
type launcher struct {
	window        *application.WebviewWindow
	mu            sync.Mutex
	shortcutError string
	screenID      string
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
	l.window.EmitEvent("launcher:show")
}

func (l *launcher) resize(height int) {
	l.mu.Lock()
	defer l.mu.Unlock()
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

func configureTray(app *application.App, l *launcher) {
	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("司南 · Command + Space")
	menu := app.NewMenu()
	menu.Add("显示司南").OnClick(func(_ *application.Context) { go l.show() })
	menu.AddSeparator()
	menu.Add("退出司南").OnClick(func(_ *application.Context) { app.Quit() })
	tray.SetMenu(menu)
}
