package kernel

import (
	"errors"
	"os"
	"path/filepath"

	"sinan/internal/plugin/manifest"
)

// System repair accepts only the two bundled trusted-component declarations.
// It never executes code from the damaged package or the replacement package.
func repairManifest(mf manifest.Manifest) bool {
	view := map[string]string{"system.settings": "settings", "system.plugin-manager": "manager"}[mf.ID]
	if view == "" || mf.Runtime.Kind != "declarative" || len(mf.Contributes.Commands) != 1 || len(mf.Contributes.UIViews) != 0 {
		return false
	}
	command := mf.Contributes.Commands[0]
	return command.ID == "open" && command.View == view && command.UIView == "" && !command.SearchOnly
}

func repairBaseline(dir string) (string, error) {
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", errors.New("无法读取待修复目录")
	}
	return packageFingerprint(dir)
}

func (m *Manager) previewSystemRepair(source string) (PackagePreview, string, error) {
	abs, err := m.packageSource(source)
	if err != nil {
		return PackagePreview{}, "", err
	}
	mf, digest, err := loadPackage(abs)
	if err != nil {
		return PackagePreview{}, "", err
	}
	if !repairManifest(mf) {
		return PackagePreview{}, "", errors.New("仅支持设置或插件管理的声明式组件包")
	}
	m.mu.Lock()
	closed, old := m.closed, m.plugins[mf.ID]
	m.mu.Unlock()
	if closed {
		return PackagePreview{}, "", errors.New("运行时已关闭")
	}
	if m.packageChangesBlocked(mf.ID) {
		return PackagePreview{}, "", errors.New("存在未恢复的事务，请修复后重启")
	}
	baseline, err := repairBaseline(filepath.Join(m.root, mf.ID))
	if err != nil {
		return PackagePreview{}, "", err
	}
	preview := PackagePreview{ID: mf.ID, Name: mf.Name, Version: mf.Version, Kind: mf.Runtime.Kind, Fingerprint: digest, InstalledFingerprint: baseline, Changed: true}
	if old != nil {
		preview.InstalledVersion = old.manifest.Version
		old.mu.Lock()
		preview.Changed = baseline != digest || old.fingerprint != digest || old.changed
		old.mu.Unlock()
	}
	return preview, abs, nil
}

// InstalledFingerprint here reviews actual disk bytes, including an invalid manifest.
func (m *Manager) PreviewSystemRepair(source string) (PackagePreview, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	preview, _, err := m.previewSystemRepair(source)
	return preview, err
}

func (m *Manager) RepairSystemPlugin(source, expected, installed string) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if len(expected) != 64 {
		return PluginInfo{}, errors.New("请先检查系统插件包")
	}
	preview, abs, err := m.previewSystemRepair(source)
	if err != nil {
		return PluginInfo{}, err
	}
	if preview.Fingerprint != expected || preview.InstalledFingerprint != installed {
		return PluginInfo{}, errors.New("插件文件已变化，请重新检查后修复")
	}
	if installed == "" {
		return m.installLocked(abs, false, expected)
	}
	work, err := os.MkdirTemp(m.root, ".upgrade-")
	if err != nil {
		return PluginInfo{}, errors.New("无法创建修复目录")
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(work)
		}
	}()
	stage := filepath.Join(work, preview.ID)
	if err = os.Mkdir(stage, 0700); err != nil {
		return PluginInfo{}, errors.New("无法创建候选目录")
	}
	if err = copyDirectory(abs, stage); err != nil {
		return PluginInfo{}, err
	}
	mf, digest, err := loadPackage(stage)
	if err != nil {
		return PluginInfo{}, err
	}
	if digest != expected || mf.ID != preview.ID || !repairManifest(mf) {
		return PluginInfo{}, errors.New("候选包已变化，请重新检查")
	}
	dest := filepath.Join(m.root, preview.ID)
	baseline, err := repairBaseline(dest)
	if err != nil || baseline != installed {
		return PluginInfo{}, errors.New("已安装文件已变化，请重新检查")
	}
	journal := upgradeJournal{ID: mf.ID, OldApproved: m.nativeApproved(mf.ID)}
	if err = writeUpgradeJournal(work, journal); err != nil {
		return PluginInfo{}, errors.New("无法记录修复事务")
	}
	cleanup = false
	m.mu.Lock()
	old := m.plugins[mf.ID]
	m.mu.Unlock()
	if old != nil {
		old.markUIChanged()
		old.stop(false)
	}
	rollback := func() (PluginInfo, error) {
		if err := m.restoreUpgrade(work, journal); err != nil {
			if old != nil {
				old.markUIChanged()
			}
			m.blockUpgrade(mf.ID)
			return PluginInfo{}, errors.New("修复失败且回滚未完成，已保留备份，请重启后重试")
		}
		return PluginInfo{}, errors.New("修复失败，已恢复原文件")
	}
	if err = os.Rename(dest, filepath.Join(work, "previous")); err != nil {
		return rollback()
	}
	if err = os.Rename(stage, dest); err != nil {
		return rollback()
	}
	if err = m.setNativeApproval(mf.ID, false); err != nil {
		return rollback()
	}
	actual, err := packageFingerprint(dest)
	if err != nil || actual != digest {
		return rollback()
	}
	journal.Committed = true
	if err = writeUpgradeJournal(work, journal); err != nil {
		return rollback()
	}
	candidate := m.packageInstance(mf, digest, dest)
	if old != nil {
		old.stop(true)
	}
	info := candidate.info()
	m.publishReplacement(candidate)
	_ = os.RemoveAll(work)
	return info, nil
}
