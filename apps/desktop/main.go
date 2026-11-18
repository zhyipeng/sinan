package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"sinan/internal/buildinfo"
	"sinan/internal/development/uiacceptance"
	"sinan/internal/hostupdate"
	"sinan/internal/kernel"
	"sinan/internal/platform"
	"sinan/internal/plugin/uihost"
	"sinan/internal/plugin/uiwindows"
	"sinan/internal/theme"
	pluginmanager "sinan/plugins/system-plugin-manager"
	settingsupdate "sinan/plugins/system-settings/update"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/icon.png
var appIcon []byte

//go:embed assets/tray-template.png
var trayIcon []byte

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	uiProbe := flag.Bool("ui-probe", false, "运行开发用 UI 隔离验证")
	pluginUI := flag.Bool("plugin-ui", true, "启用插件界面（使用 -plugin-ui=false 关闭）")
	acceptance := flag.Bool("ui-acceptance", false, "运行独立临时窗口验收环境")
	acceptancePackages := flag.String("acceptance-packages", "plugins", "验收所需系统插件源码目录")
	root := flag.String("plugins", "", "插件安装目录")
	updateSession := flag.String("update-session", "", "更新助手启动会话")
	webviewDataDir := flag.String("webview-data-dir", "", "WebView2 用户数据目录（Windows 开发隔离用；留空使用默认目录）")
	flag.Parse()
	useBundledPlugins := *root == "" && !*uiProbe && !*acceptance
	if *acceptance && (*root != "" || *uiProbe) {
		return errors.New("临时验收不能同时使用 -plugins 或 -ui-probe")
	}
	cleanupAcceptance := func() {}
	var manager *kernel.Manager
	if *acceptance {
		if runtime.GOOS != "darwin" {
			return errors.New("原生窗口验收目前仅支持 macOS")
		}
		env, err := uiacceptance.Prepare(*acceptancePackages)
		if err != nil {
			return err
		}
		defer env.Close()
		cleanupAcceptance = env.Close
		manager = env.Manager
		*pluginUI = true
		log.Print("临时窗口验收：在插件管理中找到「窗口验收」，点击「打开 输入测试」。从验收实例托盘退出后自动清理。")
		log.Print("Command + Space 若被日常实例占用，请先退出日常实例，或在验收实例的设置中修改快捷键。")
	} else {
		if *root == "" {
			dir, err := os.UserConfigDir()
			if err != nil {
				return errors.New("无法确定用户配置目录")
			}
			*root = filepath.Join(dir, "Sinan", "plugins")
		}
		var err error
		manager, err = kernel.New(*root)
		if err != nil {
			return err
		}
	}
	defer manager.Close()
	migrationNotice := ""
	if useBundledPlugins {
		if err := installMissingBundledPlugins(manager); err != nil {
			migrationNotice = err.Error()
		}
	}
	if !*uiProbe && !*acceptance {
		if err := pluginmanager.Migrate(manager); err != nil {
			if migrationNotice != "" {
				migrationNotice += "\n"
			}
			migrationNotice += err.Error()
		}
	}
	host := &HostService{manager: manager, probeEnabled: *uiProbe, migrationNotice: migrationNotice}
	host.startupError = "开发和验收实例不支持开机自启"
	host.updatesContext, host.updatesCancel = context.WithCancel(context.Background())
	defer host.updatesCancel()
	appURL := "/"
	if *acceptance {
		appURL = "/?acceptance=1"
	}
	if *uiProbe {
		appURL = "/?isolation=1"
	}
	name := "Sinan"
	if *acceptance {
		name = "Sinan Acceptance"
	}
	windowsOptions := application.WindowsOptions{WndProcInterceptor: newWindowsWndProcInterceptor(host)}
	if *webviewDataDir != "" {
		absolute, err := filepath.Abs(*webviewDataDir)
		if err != nil {
			return fmt.Errorf("无法解析 WebView2 用户数据目录：%w", err)
		}
		windowsOptions.WebviewUserDataPath = absolute
	}
	services := []application.Service{application.NewService(host)}
	if !*uiProbe && !*acceptance {
		notifier := &desktopNotifications{}
		services = append(services, application.NewService(notifier))
		manager.SetNotificationSender(notifier.send)
	}
	app := application.New(application.Options{
		Name:        name,
		Description: "插件驱动的桌面效率工具",
		Icon:        appIcon,
		Services:    services,
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		Windows:     windowsOptions,
		OnShutdown: func() {
			host.updatesCancel()
			if host.launcher != nil {
				host.launcher.closeRetention()
			}
			if host.pluginRuntimeCancel != nil {
				host.pluginRuntimeCancel()
				<-host.pluginRuntimeDone
			}
			host.cancelSearch()
			host.closePluginStore()
			if host.embedded != nil {
				uihost.CloseWarmEmbedded(application.InvokeSync)
			}
			go host.closeEmbeddedAll()
			if host.ui != nil {
				host.ui.Close()
			}
			manager.Close()
			cleanupAcceptance()
		},
	})
	if useBundledPlugins {
		host.startup = app.Autostart
		host.startupError = ""
	}
	host.quit = app.Quit
	updateExecutable, _ := os.Executable()
	if !useBundledPlugins {
		updateExecutable = ""
	}
	host.updates = settingsupdate.New(updateExecutable, buildinfo.Version, func() {
		// WebView event delivery may block. The update task only publishes state
		// and must be able to proceed without any window consuming events.
		go func() {
			if host.updatesContext.Err() == nil && host.launcher != nil {
				host.launcher.dispatch(&application.CustomEvent{Name: "host-update:changed", Sender: "system.settings"})
			}
		}()
	})
	manager.SetUIHostCall(host.callInputHost)
	host.launcher = &launcher{
		detached: map[uint]trustedWindow{},
		topOffset: func() int {
			prefs, _ := manager.LauncherPositionPreferences()
			return prefs.TopOffsetPercent
		},
	}
	if prefs, err := manager.RetentionPreferences(); err == nil {
		host.launcher.retention.preferences = prefs
	}
	host.opacity = &windowOpacityController{manager: manager, apply: host.launcher.setOpacity}
	var shellSequence uint
	newShell := func(url string, hidden bool) *application.WebviewWindow {
		shellSequence++
		themeState, themeError := manager.ThemeSettings()
		if themeError != nil {
			themeState = theme.Builtins("system", 0)
		}
		appearance := themeState.Mode
		if appearance == "system" {
			appearance = "light"
			// App.Run has not initialized Wails' dispatcher on first creation.
			// The system preference read is thread-safe and requires no window.
			if platform.SystemDark() {
				appearance = "dark"
			}
		}
		macAppearance := application.DefaultAppearance
		if themeState.Mode == "light" {
			macAppearance = application.NSAppearanceNameAqua
		}
		if themeState.Mode == "dark" {
			macAppearance = application.NSAppearanceNameDarkAqua
		}
		window := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: fmt.Sprintf("shell-%d", shellSequence), Title: "司南", Width: launcherWidth, Height: launcherMinHeight,
			MinWidth: launcherWidth, MinHeight: launcherMinHeight,
			Frameless: true, DisableResize: true, AlwaysOnTop: true,
			Hidden:          hidden,
			InitialPosition: application.WindowCentered,
			Mac: application.MacWindow{
				// A non-activating panel can take keyboard focus over another app's
				// fullscreen Space without activating Sinan and switching desktops.
				WindowClass: application.MacWindowClassPanel,
				Appearance:  macAppearance,
				PanelPreferences: application.MacPanelPreferences{
					NonActivating: true,
				},
				WindowLevel: application.MacWindowLevelFloating,
				CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
					application.MacWindowCollectionBehaviorFullScreenAuxiliary,
				TitleBar: application.MacTitleBar{Hide: true},
			},
			BackgroundColour: themeColour(themeState, appearance), URL: url,
		})
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
			go host.restoreWindowOpacity(window)
			if host.embedded != nil && runtime.GOOS == "windows" {
				go func() {
					if host.launcher.isMain(window) {
						uihost.WarmEmbedded(window.NativeWindow, application.InvokeSync)
					}
				}()
			}
		})
		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			if host.launcher.isMain(window) {
				event.Cancel()
				host.launcher.hide(window)
			} else {
				go host.releaseEmbedded(window)
				host.launcher.forget(window)
			}
			host.stopRecording(window)
		})
		window.OnWindowEvent(events.Common.WindowLostFocus, func(_ *application.WindowEvent) {
			if host.launcher.shouldHideOnBlur(window) {
				host.cancelSearch()
				host.launcher.hide(window)
			}
			host.stopRecording(window)
		})

		return window
	}
	startupPrefs, startupErr := manager.StartupPreferences()
	// Keep the search panel hidden until the launcher can position it.
	window := newShell(appURL, !*uiProbe)
	host.launcher.window = window
	silent := startupErr == nil && startupPrefs.Silent && !*acceptance
	if !*uiProbe && !silent {
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
			host.launcher.showAtStartup(window)
		})
	}
	host.launcher.newWindow = func() *application.WebviewWindow { return newShell("/", true) }
	if *pluginUI && !*uiProbe && uihost.EmbeddedSupported {
		host.embedded = &embeddedViews{entries: map[*application.WebviewWindow]*embeddedEntry{}, sequence: map[*application.WebviewWindow]uint64{}}
	}
	if *pluginUI && !*uiProbe && host.embedded == nil {
		executable, err := os.Executable()
		if !uihost.Supported {
			host.uiError = "当前平台尚未实现隔离插件界面"
		} else if err != nil {
			host.uiError = "无法确定 UI 客体程序位置"
		} else {
			controller, err := uiwindows.New(manager, filepath.Join(filepath.Dir(executable), "sinan-ui-host"), func(event uiwindows.Event) {
				if event.ResetToSearch {
					go host.showMain()
					return
				}
				if event.OpenSettings {
					_ = host.openSettings()
					return
				}
				if event.ReturnToSearch {
					go host.showMain()
				}
				if !event.Detached {
					host.launcher.dispatch(&application.CustomEvent{Name: "ui:closed", Data: event, Sender: "kernel.ui"})
				}
			})
			if err != nil {
				host.uiError = err.Error()
			} else {
				host.ui = controller
				if state, err := manager.ThemeSettings(); err == nil {
					controller.SetTheme(state)
				}
				defer controller.Close()
			}
		}
	}
	if !*uiProbe {
		tray := configureTray(app, host.showMain)
		host.shortcuts = newShortcutController(manager, app.GlobalShortcut, host.toggleMain, func(value string) {
			host.captureShortcut(value)
		}, func(state ShortcutState) {
			host.launcher.setShortcutError(state.Error)
			host.updateAltSpaceHotkey()
			if *acceptance {
				tray.SetTooltip("司南 · 临时验收 · " + state.Shortcut)
			} else {
				tray.SetTooltip("司南 · " + state.Shortcut)
			}
			host.launcher.dispatch(&application.CustomEvent{Name: "shortcut:changed", Data: state, Sender: "kernel.shortcuts"})
		})
		host.detachShortcuts = newShortcutControllerWithDefault(windowPreferences{manager}, localShortcutRegistry{}, nil, nil, func(state ShortcutState) {
			host.updateAltSpaceHotkey()
			host.setEmbeddedShortcut(state.Active, state.Revision)
			if host.ui != nil {
				host.ui.SetDetachShortcut(state.Active)
			}
			host.launcher.dispatch(&application.CustomEvent{Name: "detach-shortcut:changed", Data: state, Sender: "kernel.shortcuts"})
		}, defaultDetachShortcut())
		if host.ui != nil {
			host.ui.SetDetachShortcut(host.detachShortcuts.State().Shortcut)
		}
		// Register after startup so native conflicts are returned synchronously.
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
			host.shortcuts.Start()
			host.startDetachShortcuts()
			host.startPluginRuntime(app.GlobalShortcut)
			if *updateSession != "" && useBundledPlugins {
				if err := hostupdate.ConfirmStartup(*updateSession, buildinfo.Version, updateExecutable); err != nil {
					log.Print("更新就绪回执失败")
				}
			}
			if host.updates.State().Supported {
				go func() {
					timer := time.NewTimer(3 * time.Second)
					defer timer.Stop()
					select {
					case <-timer.C:
						_, _ = host.updates.Check(host.updatesContext)
					case <-host.updatesContext.Done():
					}
				}()
			}
		})
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(_ *application.ApplicationEvent) {
			go host.showMain()
		})
	}
	if err := app.Run(); err != nil {
		return errors.New("桌面宿主运行失败")
	}
	return nil
}
