package main

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"sinan/internal/kernel"
)

// Shipped plugin bytes are part of the host executable, not discovered from a
// writable directory next to it. Plugin business remains in the plugin layer.
//
//go:embed all:bundled
var bundledPlugins embed.FS

func installMissingBundledPlugins(manager *kernel.Manager) error {
	return installMissingPlugins(manager, bundledPlugins)
}

func installMissingPlugins(manager *kernel.Manager, bundle fs.FS) error {
	var failures []error
	installed := map[string]bool{}
	for _, plugin := range manager.Snapshot().Plugins {
		installed[plugin.ID] = true
	}
	for _, id := range []string{"system.app-search", "system.settings", "system.plugin-manager"} {
		if installed[id] {
			continue
		}
		source, err := fs.Sub(bundle, "bundled/"+id)
		if err != nil {
			failures = append(failures, errors.New("无法读取内置插件："+id))
			continue
		}
		if _, err = fs.Stat(source, "manifest.json"); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			failures = append(failures, errors.New("无法读取内置插件清单："+id))
			continue
		}
		work, err := os.MkdirTemp("", "sinan-bundled-")
		if err != nil {
			failures = append(failures, errors.New("无法准备内置插件："+id))
			continue
		}
		err = installBundledPlugin(manager, source, work, id == "system.app-search")
		os.RemoveAll(work)
		if err != nil {
			failures = append(failures, errors.New("内置插件初始化未完成，请通过恢复入口检查："+id))
		}
	}
	return errors.Join(failures...)
}

func installBundledPlugin(manager *kernel.Manager, source fs.FS, work string, native bool) error {
	err := fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(work, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0600)
		if strings.HasPrefix(path, "backend/") {
			mode = 0700
		}
		return os.WriteFile(target, data, mode)
	})
	if err != nil {
		return err
	}
	_, err = manager.Install(work, native)
	return err
}
