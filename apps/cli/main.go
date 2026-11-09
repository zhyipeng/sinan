package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"sinan/internal/kernel"
	"sinan/internal/search"
)

func run() error {
	config, err := os.UserConfigDir()
	if err != nil {
		return errors.New("无法确定配置目录")
	}
	root := flag.String("plugins", filepath.Join(config, "Sinan", "plugins"), "插件安装目录")
	trusted := flag.Bool("trust-native", false, "信任并安装可访问当前用户系统资源的原生插件")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		return errors.New("用法: sinanctl [-plugins 路径] [-trust-native] list|install 目录|uninstall ID|run ID 命令 [输入]|search 查询|clear-history|settings ID|set-settings ID revision JSON|inspect 目录|upgrade 目录|reload ID|enable ID|disable ID|repair-system 目录")
	}
	m, err := kernel.New(*root)
	if err != nil {
		return err
	}
	defer m.Close()
	var result any
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return errors.New("list 不接受其他参数")
		}
		result = m.Snapshot()
	case "install":
		if len(args) != 2 {
			return errors.New("install 需要插件目录")
		}
		result, err = m.Install(args[1], *trusted)
	case "uninstall":
		if len(args) != 2 {
			return errors.New("uninstall 需要插件 ID")
		}
		err = m.Uninstall(args[1])
		result = "插件已卸载"
	case "search":
		if len(args) != 2 {
			return errors.New("search 需要查询文字")
		}
		var outputErr error
		coordinator := search.Coordinator{Backend: m}
		coordinator.Query(context.Background(), "cli", args[1], func(update search.Update) {
			if outputErr == nil {
				outputErr = json.NewEncoder(os.Stdout).Encode(update)
			}
		})
		return outputErr
	case "run":
		if len(args) < 3 || len(args) > 4 {
			return errors.New("run 需要插件 ID、命令 ID 和可选输入")
		}
		input := ""
		if len(args) == 4 {
			input = args[3]
		}
		result, err = m.Execute(context.Background(), args[1], args[2], input)
	case "clear-history":
		if len(args) != 1 {
			return errors.New("clear-history 不接受其他参数")
		}
		err = m.ClearUsage()
		result = "使用历史已清除"
	case "settings":
		if len(args) != 2 {
			return errors.New("settings 需要插件 ID")
		}
		result, err = m.PluginSettings(args[1])
	case "set-settings":
		if len(args) != 4 {
			return errors.New("set-settings 需要插件 ID、当前 revision 和 JSON 对象")
		}
		revision, parseErr := strconv.ParseUint(args[2], 10, 64)
		var values map[string][]string
		if parseErr != nil || json.Unmarshal([]byte(args[3]), &values) != nil {
			return errors.New("设置参数格式无效")
		}
		result, err = m.SavePluginSettings(args[1], revision, values)
	case "inspect":
		if len(args) != 2 {
			return errors.New("inspect 需要插件目录")
		}
		result, err = m.PreviewPackage(args[1])
	case "upgrade":
		if len(args) != 2 {
			return errors.New("upgrade 需要插件目录")
		}
		result, err = m.Upgrade(args[1], *trusted)
	case "repair-system":
		if len(args) != 2 {
			return errors.New("repair-system 需要系统插件目录")
		}
		var preview kernel.PackagePreview
		preview, err = m.PreviewSystemRepair(args[1])
		if err == nil {
			result, err = m.RepairSystemPlugin(args[1], preview.Fingerprint, preview.InstalledFingerprint)
		}
	case "reload":
		if len(args) != 2 {
			return errors.New("reload 需要插件 ID")
		}
		result, err = m.Reload(args[1])
	case "enable", "disable":
		if len(args) != 2 {
			return errors.New("enable / disable 需要插件 ID")
		}
		result, err = m.SetPluginEnabled(args[1], args[0] == "enable")
	default:
		return errors.New("未知操作")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
