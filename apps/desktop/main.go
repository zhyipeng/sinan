package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sinan/internal/kernel"
	"sinan/internal/plugin/uiwindows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/icon.png
var appIcon []byte

//go:embed assets/tray-template.png
var trayIcon []byte

func main() {
	uiProbe := flag.Bool("ui-probe", false, "运行开发用 UI 隔离验证")
	pluginUI := flag.Bool("plugin-ui", false, "启用开发期独立插件窗口")
	root := flag.String("plugins", "", "插件安装目录")
	flag.Parse()
	if *root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal("无法确定用户配置目录")
		}
		*root = filepath.Join(dir, "Sinan", "plugins")
	}
	manager, err := kernel.New(*root)
	if err != nil {
		log.Fatal("无法初始化插件运行时：", err)
	}
	defer manager.Close()
	host := &HostService{manager: manager, probeEnabled: *uiProbe}
	appURL := "/"
	if *uiProbe {
		appURL = "/?isolation=1"
	}
	app := application.New(application.Options{
		Name:        "Sinan",
		Description: "插件驱动的桌面效率工具",
		Icon:        appIcon,
		Services:    []application.Service{application.NewService(host)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		OnShutdown: func() {
			host.cancelSearch()
			if host.ui != nil {
				host.ui.Close()
			}
			manager.Close()
		},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "launcher", Title: "司南", Width: launcherWidth, Height: launcherMinHeight,
		MinWidth: launcherWidth, MinHeight: launcherMinHeight,
		Frameless: true, DisableResize: true, AlwaysOnTop: true,
		HideOnFocusLost: true, HideOnEscape: true,
		InitialPosition: application.WindowCentered,
		Mac: application.MacWindow{
			// A non-activating panel can take keyboard focus over another app's
			// fullscreen Space without activating Sinan and switching desktops.
			WindowClass: application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true,
			},
			WindowLevel: application.MacWindowLevelFloating,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
			TitleBar: application.MacTitleBar{Hide: true},
		},
		BackgroundColour: application.NewRGB(246, 247, 251), URL: appURL,
	})
	host.launcher = &launcher{window: window}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		window.Hide()
	})
	window.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) {
		host.cancelSearch()
		if host.shortcuts != nil {
			go func() { _ = host.shortcuts.Record(false) }()
		}
	})
	if *pluginUI && !*uiProbe {
		executable, err := os.Executable()
		if runtime.GOOS != "darwin" {
			host.uiError = "独立插件窗口目前仅支持 macOS"
		} else if err != nil {
			host.uiError = "无法确定 UI 客体程序位置"
		} else {
			controller, err := uiwindows.New(manager, filepath.Join(filepath.Dir(executable), "sinan-ui-host"), func(event uiwindows.Event) {
				window.DispatchWailsEvent(&application.CustomEvent{Name: "ui:closed", Data: event, Sender: "kernel.ui"})
			})
			if err != nil {
				host.uiError = err.Error()
			} else {
				host.ui = controller
				defer controller.Close()
			}
		}
	}
	if !*uiProbe {
		tray := configureTray(app, host.launcher)
		host.shortcuts = newShortcutController(manager, app.GlobalShortcut, host.launcher.toggle, func(value string) {
			window.DispatchWailsEvent(&application.CustomEvent{Name: "shortcut:recorded", Data: value, Sender: "kernel.shortcuts"})
		}, func(state ShortcutState) {
			host.launcher.setShortcutError(state.Error)
			tray.SetTooltip("司南 · " + state.Shortcut)
			window.DispatchWailsEvent(&application.CustomEvent{Name: "shortcut:changed", Data: state, Sender: "kernel.shortcuts"})
		})
		// Register after startup so native conflicts are returned synchronously.
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) { host.shortcuts.Start() })
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(_ *application.ApplicationEvent) {
			go host.launcher.show()
		})
	}
	if err := app.Run(); err != nil {
		log.Fatal("桌面宿主运行失败")
	}
}
