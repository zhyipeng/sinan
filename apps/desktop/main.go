package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	app := application.New(application.Options{
		Name:        "Sinan",
		Description: "插件驱动的桌面效率工具",
		Services:    []application.Service{application.NewService(&HostService{manager: manager})},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		OnShutdown:  manager.Close,
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "司南", Width: 760, Height: 580, MinWidth: 560, MinHeight: 440,
		BackgroundColour: application.NewRGB(246, 247, 251), URL: "/",
	})
	if err := app.Run(); err != nil {
		log.Fatal("桌面宿主运行失败")
	}
}
