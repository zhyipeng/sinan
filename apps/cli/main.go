package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
		return errors.New("用法: sinanctl [-plugins 路径] [-trust-native] list|install 目录|uninstall ID|run ID 命令 [输入]|search 查询")
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
