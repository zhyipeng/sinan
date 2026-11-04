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
	var screen *application.Screen
	if id, ok := platform.CursorScreenID(); ok {
		screen = application.GetScreenByID(id)
	}
	if screen == nil {
		screen, _ = l.window.GetScreen()
	}
	if screen != nil {
		area := screen.WorkArea
		l.window.SetPosition(area.X+(area.Width-launcherWidth)/2, area.Y+min(160, area.Height/5))
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
	if screen, err := l.window.GetScreen(); err == nil && screen != nil {
		area := screen.WorkArea
		height = min(height, max(launcherMinHeight, area.Y+area.Height-y-16))
	}
	// macOS changes the origin when resizing; restore the top-left anchor.
	l.window.SetSize(launcherWidth, height)
	l.window.SetPosition(x, y)
}

func (l *launcher) setShortcutError(message string) {
	l.mu.Lock()
	l.shortcutError = message
	l.mu.Unlock()
}
func (l *launcher) status() string { l.mu.Lock(); defer l.mu.Unlock(); return l.shortcutError }

func configureTray(app *application.App, l *launcher) {
	tray := app.SystemTray.New()
	// A text label is deliberately used as a dependable first tray entry.
	tray.SetLabel("司南")
	tray.SetTooltip("司南 · Command + Space")
	menu := app.NewMenu()
	menu.Add("显示司南").OnClick(func(_ *application.Context) { go l.show() })
	menu.AddSeparator()
	menu.Add("退出司南").OnClick(func(_ *application.Context) { app.Quit() })
	tray.SetMenu(menu)
}
