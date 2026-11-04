package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sinan/internal/kernel"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
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
		log.Fatal("无法初始化插件运行时")
	}
	defer manager.Close()
	host := &HostService{manager: manager}
	app := application.New(application.Options{
		Name:        "Sinan",
		Description: "插件驱动的桌面效率工具",
		Services:    []application.Service{application.NewService(host)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		OnShutdown:  manager.Close,
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "launcher", Title: "司南", Width: launcherWidth, Height: launcherMinHeight,
		MinWidth: launcherWidth, MinHeight: launcherMinHeight,
		Frameless: true, DisableResize: true, AlwaysOnTop: true,
		HideOnFocusLost: true, HideOnEscape: true,
		InitialPosition: application.WindowCentered,
		Mac: application.MacWindow{
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary,
			TitleBar: application.MacTitleBar{Hide: true},
		},
		BackgroundColour: application.NewRGB(246, 247, 251), URL: "/",
	})
	host.launcher = &launcher{window: window}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		window.Hide()
	})
	configureTray(app, host.launcher)
	// Register after startup so OS conflicts are returned to us rather than only logged.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		if err := app.GlobalShortcut.Register("CmdOrCtrl+Space", host.launcher.show); err != nil {
			log.Print("全局快捷键注册失败，仍可通过托盘显示司南")
			host.launcher.setShortcutError("Command + Space 注册失败，请检查系统聚焦或其他应用的快捷键占用。可从托盘打开司南。")
		}
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(_ *application.ApplicationEvent) {
		go host.launcher.show()
	})
	if err := app.Run(); err != nil {
		log.Fatal("桌面宿主运行失败")
	}
}
