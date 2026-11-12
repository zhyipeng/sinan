// Package pluginmanager owns the bundled plugin center and its identity migration.
package pluginmanager

import (
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"sinan/internal/kernel"
)

//go:embed manifest.json icon.svg
var bundled embed.FS

// Fingerprints of the unmodified 0.6.0 manager and 0.1.0 store, in source-tree
// and manifest/icon-only installations. Unknown package bytes are never replaced.
var previousManagers = map[string]bool{
	"416f81f36e6cdf39546c93a1d9ef62dd2909a6408a02b9919ebc5ad0b11eea4c": true,
	"115ca12762c85f78e7bda3b5fedabe8eebd84abfdba190bdd77b1f9602557fdb": true,
}
var previousStores = map[string]bool{
	"307bf0815929d12b643c5a0483801e5cfe4c54d481c22a6a01960d53ad4e68f6": true,
	"2bbd1ad34760ded3ca52c91d5f3719919130a1bac37b43c7dbf76938ce235581": true,
}

type MigrationBackend interface {
	Snapshot() kernel.Snapshot
	PreviewPackage(string) (kernel.PackagePreview, error)
	ApplyPackage(string, bool, string, string) (kernel.PluginInfo, error)
	UninstallChecked(string, string) error
}

// Migrate upgrades only known bundled bytes. It leaves settings, private data,
// caches and usage records intact, including those of the retired store.
func Migrate(backend MigrationBackend) error {
	var manager, store *kernel.PluginInfo
	for _, plugin := range backend.Snapshot().Plugins {
		p := plugin
		switch p.ID {
		case "system.plugin-manager":
			manager = &p
		case "system.plugin-store":
			store = &p
		}
	}
	if manager == nil && store == nil {
		return nil
	}
	work, err := os.MkdirTemp("", "sinan-plugin-center-")
	if err != nil {
		return errors.New("无法准备插件中心迁移")
	}
	defer os.RemoveAll(work)
	candidate := filepath.Join(work, "manager")
	if err = os.Mkdir(candidate, 0700); err != nil {
		return errors.New("无法准备插件中心迁移")
	}
	for _, name := range []string{"manifest.json", "icon.svg"} {
		data, readErr := bundled.ReadFile(name)
		if readErr != nil {
			return errors.New("无法读取插件中心内置包")
		}
		if err = os.WriteFile(filepath.Join(candidate, name), data, 0600); err != nil {
			return errors.New("无法准备插件中心迁移")
		}
	}
	var current struct {
		Version string `json:"version"`
	}
	data, _ := bundled.ReadFile("manifest.json")
	if err = json.Unmarshal(data, &current); err != nil {
		return errors.New("插件中心内置包无效")
	}
	if manager == nil || manager.Version != current.Version {
		preview, previewErr := backend.PreviewPackage(candidate)
		if previewErr != nil {
			return errors.New("无法检查插件中心迁移，请通过恢复入口处理")
		}
		if manager != nil && (manager.Development || manager.Changed || !previousManagers[preview.InstalledFingerprint]) {
			return errors.New("插件管理包不是已知内置版本，已保留原文件，请从恢复入口升级插件中心")
		}
		if _, err = backend.ApplyPackage(candidate, false, preview.Fingerprint, preview.InstalledFingerprint); err != nil {
			return errors.New("插件中心迁移未完成，原插件商店已保留，请通过恢复入口重试")
		}
	}
	if store == nil {
		return nil
	}
	for _, p := range backend.Snapshot().Plugins {
		if p.ID == "system.plugin-manager" {
			valid := p.Kind == "declarative" && !p.Disabled && !p.Changed
			found := false
			for _, command := range p.Commands {
				if command.ID == "open" && command.View == "manager" {
					found = true
				}
			}
			if !valid || !found {
				return errors.New("插件中心声明不可用，旧插件商店已保留，请通过恢复入口处理")
			}
		}
	}
	if store.Development || store.Changed {
		return errors.New("旧插件商店存在本地修改，已保留，请在插件中心检查后卸载")
	}
	// A declarative probe obtains the installed fingerprint through the existing
	// package preview API; it is never installed or executed.
	probe := filepath.Join(work, "store")
	if err = os.Mkdir(probe, 0700); err != nil {
		return errors.New("无法检查旧插件商店")
	}
	declaration := []byte(`{"id":"system.plugin-store","name":"插件商店","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"open","title":"插件商店","view":"store"}]},"permissions":[],"activationEvents":["onCommand:open"]}`)
	if err = os.WriteFile(filepath.Join(probe, "manifest.json"), declaration, 0600); err != nil {
		return errors.New("无法检查旧插件商店")
	}
	preview, err := backend.PreviewPackage(probe)
	if err != nil || !previousStores[preview.InstalledFingerprint] {
		return errors.New("旧插件商店不是已知内置包，已保留，请在插件中心检查后卸载")
	}
	if err = backend.UninstallChecked(store.ID, preview.InstalledFingerprint); err != nil {
		return errors.New("旧插件商店已变化或无法移除，已保留，请在插件中心检查")
	}
	return nil
}
