// Package uiacceptance prepares disposable manual-acceptance instances only.
package uiacceptance

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

//go:embed fixture/ui/*
var fixture embed.FS

const PluginID = "dev.ui-acceptance"

type Environment struct {
	Directory string
	Manager   *kernel.Manager
	once      sync.Once
}

func Prepare(packages string) (*Environment, error) {
	dir, err := os.MkdirTemp("", "sinan-ui-acceptance-")
	if err != nil {
		return nil, errors.New("无法创建临时验收目录")
	}
	env := &Environment{Directory: dir}
	failed := true
	defer func() {
		if failed {
			env.Close()
		}
	}()
	env.Manager, err = kernel.New(filepath.Join(dir, "plugins"))
	if err != nil {
		return nil, errors.New("无法创建验收运行时")
	}
	for _, id := range []string{"system.settings", "system.plugin-manager"} {
		name := "system-settings"
		if id == "system.plugin-manager" {
			name = "system-plugin-manager"
		}
		source := filepath.Join(packages, name)
		mf, err := manifest.Load(source)
		if err != nil || mf.ID != id || mf.Runtime.Kind != "declarative" {
			return nil, errors.New("无法读取验收所需系统插件，请从项目根目录启动")
		}
		if _, err = env.Manager.Install(source, false); err != nil {
			return nil, errors.New("无法安装验收所需系统插件")
		}
	}
	source := filepath.Join(dir, "fixture")
	assets, err := fs.Sub(fixture, "fixture")
	if err != nil {
		return nil, errors.New("无法读取验收资源")
	}
	if err = os.CopyFS(source, assets); err != nil {
		return nil, errors.New("无法准备验收页面")
	}
	mf := manifest.Manifest{ID: PluginID, Name: "窗口验收", Version: "0.1.0", APIVersion: protocol.APIVersion, Runtime: manifest.Runtime{Kind: "declarative"}, Contributes: manifest.Contributions{
		Commands: []manifest.Command{{ID: "open", Title: "窗口验收", Keywords: []string{"window", "验收", "独立窗口"}, UIView: "input"}},
		UIViews:  []protocol.UIView{{Title: "输入测试", ID: "input", Entry: "index.html", BridgeVersion: protocol.UIBridgeVersion}},
	}}
	data, err := json.Marshal(mf)
	if err != nil || os.WriteFile(filepath.Join(source, "manifest.json"), data, 0600) != nil {
		return nil, errors.New("无法准备验收声明")
	}
	if _, err = env.Manager.Install(source, false); err != nil {
		return nil, errors.New("无法安装验收页面")
	}
	failed = false
	return env, nil
}

// Close must run after the window controller has reaped its guests.
func (e *Environment) Close() {
	e.once.Do(func() {
		if e.Manager != nil {
			e.Manager.Close()
		}
		_ = os.RemoveAll(e.Directory)
	})
}
