package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"sinan/internal/plugin/manifest"
)

type upgradeJournal struct {
	ID          string `json:"id"`
	Committed   bool   `json:"committed"`
	OldApproved bool   `json:"oldApproved"`
}

func (m *Manager) Install(source string, trustNative bool) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return m.installLocked(source, trustNative, "")
}

// ApplyPackage binds a UI review to the exact staged package bytes.
func (m *Manager) ApplyPackage(source string, trustNative bool, expected string, installed string) (PluginInfo, error) {
	if len(expected) != 64 {
		return PluginInfo{}, errors.New("请先检查插件包")
	}
	return m.applyPackage(source, trustNative, expected, false, &installed)
}
func (m *Manager) Upgrade(source string, trustNative bool) (PluginInfo, error) {
	return m.applyPackage(source, trustNative, "", true, nil)
}
func (m *Manager) applyPackage(source string, trustNative bool, expected string, requireExisting bool, expectedInstalled *string) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	defer m.invalidateDependents()
	abs, err := m.packageSource(source)
	if err != nil {
		return PluginInfo{}, err
	}
	mf, err := manifest.Load(abs)
	if err != nil {
		return PluginInfo{}, err
	}
	if err = m.validateDependencyGraph(mf); err != nil {
		return PluginInfo{}, err
	}
	if m.packageChangesBlocked(mf.ID) {
		return PluginInfo{}, errors.New("存在未恢复的升级事务，请修复后重启")
	}
	old, lookupErr := m.lookup(mf.ID)
	if expectedInstalled != nil {
		actual := ""
		if old != nil {
			actual = old.fingerprint
		}
		if actual != *expectedInstalled {
			return PluginInfo{}, errors.New("已安装版本已变化，请重新检查插件包")
		}
	}
	if lookupErr != nil {
		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed || requireExisting {
			return PluginInfo{}, lookupErr
		}
		return m.installLocked(abs, trustNative, expected)
	}
	if old.development {
		return PluginInfo{}, errors.New("开发链接请重新构建并重载；需普通安装时先卸载链接")
	}
	if info, err := os.Lstat(old.dir); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return PluginInfo{}, errors.New("安装目录已被替换，请还原后重试")
	}
	installedHash, hashErr := packageFingerprint(old.dir)
	if hashErr != nil || installedHash != old.fingerprint {
		old.markUIChanged()
		return PluginInfo{}, errors.New("已安装文件已变化，请先重载或还原后升级")
	}
	if mf.Runtime.Kind == "process" && !trustNative {
		return PluginInfo{}, errors.New("升级原生插件需要重新确认信任")
	}
	work, err := os.MkdirTemp(m.root, ".upgrade-")
	if err != nil {
		return PluginInfo{}, errors.New("无法创建升级目录")
	}
	// Once a journal exists, failed recovery must leave it available for the next startup.
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(work)
		}
	}()
	stage := filepath.Join(work, mf.ID)
	if err = os.Mkdir(stage, 0700); err != nil {
		return PluginInfo{}, errors.New("无法创建候选目录")
	}
	if err = copyDirectory(abs, stage); err != nil {
		return PluginInfo{}, err
	}
	copied, digest, err := loadPackage(stage)
	if err != nil {
		return PluginInfo{}, err
	}
	if !reflect.DeepEqual(copied, mf) {
		return PluginInfo{}, errors.New("插件清单正在变化，请重新检查")
	}
	if expected != "" && expected != digest {
		return PluginInfo{}, errors.New("插件包已变化，请重新检查后再升级")
	}
	if digest == old.fingerprint {
		old.mu.Lock()
		info := old.info()
		old.mu.Unlock()
		return info, nil
	}
	oldApproved := m.nativeApproved(mf.ID)
	journal := upgradeJournal{ID: mf.ID, OldApproved: oldApproved}
	if err = writeUpgradeJournal(work, journal); err != nil {
		return PluginInfo{}, errors.New("无法记录升级事务")
	}
	cleanup = false
	old.mu.Lock()
	wasRunning := old.process != nil && old.process.Alive()
	old.mu.Unlock()
	old.stop(true)
	backup := filepath.Join(work, "previous")
	if err = os.Rename(old.dir, backup); err != nil {
		// The files never moved; restore the registry using a fresh instance token.
		replacement := m.packageInstance(old.manifest, old.fingerprint, old.dir)
		m.publishReplacement(replacement)
		if wasRunning {
			_ = m.activateCandidate(replacement)
		}
		if cleanupErr := os.RemoveAll(work); cleanupErr != nil {
			replacement.stop(true)
			m.blockUpgrade(mf.ID)
			return PluginInfo{}, errors.New("旧插件文件保留，但事务清理未完成，请修复后重启")
		}
		return PluginInfo{}, errors.New("无法备份旧插件，升级未应用")
	}
	rollback := func() (PluginInfo, error) {
		if err := m.restoreUpgrade(work, journal); err != nil {
			m.blockUpgrade(mf.ID)
			return PluginInfo{}, errors.New("升级失败且恢复未完成，已保留备份，请重启后重试")
		}
		restored := m.packageInstance(old.manifest, old.fingerprint, old.dir)
		m.publishReplacement(restored)
		if wasRunning {
			if err := m.activateCandidate(restored); err != nil {
				return PluginInfo{}, errors.New("升级失败，旧文件已恢复，插件需重新加载")
			}
		}
		return PluginInfo{}, errors.New("升级失败，已恢复旧版本")
	}
	if err = os.Rename(stage, old.dir); err != nil {
		return rollback()
	}
	if err = m.setNativeApproval(mf.ID, copied.Runtime.Kind == "process"); err != nil {
		return rollback()
	}
	candidate := m.packageInstance(copied, digest, old.dir)
	if err = m.activateCandidate(candidate); err != nil {
		candidate.stop(true)
		return rollback()
	}
	actual, hashErr := packageFingerprint(old.dir)
	if hashErr != nil || actual != digest {
		candidate.stop(true)
		return rollback()
	}
	journal.Committed = true
	if err = writeUpgradeJournal(work, journal); err != nil {
		candidate.stop(true)
		return rollback()
	}
	info := candidate.info()
	m.publishReplacement(candidate)
	_ = os.RemoveAll(work)
	return info, nil
}
func (m *Manager) packageInstance(mf manifest.Manifest, digest, dir string) *plugin {
	development := dir != filepath.Join(m.root, mf.ID)
	return &plugin{manager: m, development: development, nativeTrusted: developmentApproved(m.root, mf.ID, dir), disabled: m.enablementErr != nil || m.disabled[mf.ID], manifest: mf, fingerprint: digest, dir: dir, state: "installed", settings: settingsValues(mf, m.settings.Get(mf.ID).Values)}
}
func (m *Manager) publishReplacement(p *plugin) {
	m.mu.Lock()
	m.plugins[p.manifest.ID] = p
	m.mu.Unlock()
}
func (m *Manager) activateCandidate(p *plugin) error {
	if p.disabled || p.manifest.Runtime.Kind != "process" || p.development && !p.nativeTrusted {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.ensureProcess(ctx, m.root); err != nil {
		return err
	}
	p.mu.Lock()
	capable := p.process != nil && p.process.Alive() && (len(p.manifest.Contributes.SearchProviders) == 0 || (slices.Contains(p.capabilities, "search.query") && slices.Contains(p.capabilities, "$/cancelRequest")))
	p.mu.Unlock()
	if !capable {
		p.stop(true)
		return errors.New("搜索能力协商失败")
	}
	return nil
}
func (m *Manager) Reload(id string) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return m.reloadLocked(id)
}
func (m *Manager) reloadLocked(id string) (PluginInfo, error) {
	if m.packageChangesBlocked(id) {
		return PluginInfo{}, errors.New("存在未恢复的升级事务，请修复后重启")
	}
	old, err := m.lookup(id)
	if err != nil {
		return PluginInfo{}, err
	}
	if old.development {
		target, err := m.developmentTarget(id)
		if err != nil || target != old.dir {
			return PluginInfo{}, errors.New("开发链接目标已变化，请卸载链接后重新登记")
		}
	}
	mf, digest, err := loadPackage(old.dir)
	if err != nil || mf.ID != id {
		old.markUIChanged()
		return PluginInfo{}, errors.New("插件包无效，已保留当前实例")
	}
	if digest != old.fingerprint {
		old.markUIChanged()
	}
	candidate := m.packageInstance(mf, digest, old.dir)
	if err = m.activateCandidate(candidate); err != nil {
		candidate.stop(true)
		return PluginInfo{}, errors.New("重载校验失败，已保留当前实例")
	}
	current, err := packageFingerprint(old.dir)
	if err != nil || current != digest {
		candidate.stop(true)
		return PluginInfo{}, errors.New("插件文件正在变化，已取消重载")
	}
	old.stop(true)
	info := candidate.info()
	m.publishReplacement(candidate)
	return info, nil
}
func (m *Manager) nativeApproved(id string) bool {
	data, err := os.ReadFile(filepath.Join(m.root, ".trusted", id))
	return err == nil && string(data) == "trusted-native-v1\n"
}
func (m *Manager) setNativeApproval(id string, approved bool) error {
	path := filepath.Join(m.root, ".trusted", id)
	if !approved {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("trusted-native-v1\n"), 0600)
}
func writeUpgradeJournal(work string, journal upgradeJournal) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(work, ".journal-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(work, "transaction.json"))
}
func (m *Manager) restoreUpgrade(work string, journal upgradeJournal) error {
	dest := filepath.Join(m.root, journal.ID)
	backup := filepath.Join(work, "previous")
	info, err := os.Lstat(backup)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("invalid backup")
		}
		if _, err := os.Lstat(dest); err == nil {
			failed, tempErr := os.MkdirTemp(work, "failed-")
			if tempErr != nil {
				return tempErr
			}
			if err = os.Remove(failed); err != nil {
				return err
			}
			if err = os.Rename(dest, failed); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(backup, dest); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = m.setNativeApproval(journal.ID, journal.OldApproved); err != nil {
		return err
	}
	return os.RemoveAll(work)
}
func (m *Manager) recoverUpgrades() []string {
	diagnostics := []string{}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return diagnostics
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".upgrade-") || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		work := filepath.Join(m.root, entry.Name())
		file, err := os.Open(filepath.Join(work, "transaction.json"))
		var data []byte
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(file, 4097))
			file.Close()
		}
		if os.IsNotExist(err) {
			_ = os.RemoveAll(work)
			continue
		}
		var journal upgradeJournal
		if err != nil || len(data) > 4096 || json.Unmarshal(data, &journal) != nil || !manifest.ValidID(journal.ID) {
			m.recoveryBlocked = true
			diagnostics = append(diagnostics, "升级事务无效，已保留现场")
			continue
		}
		if journal.Committed {
			if err = os.RemoveAll(work); err != nil {
				diagnostics = append(diagnostics, "旧插件备份清理失败")
			}
			continue
		}
		if err = m.restoreUpgrade(work, journal); err != nil {
			m.blockedUpgrades[journal.ID] = true
			diagnostics = append(diagnostics, "插件升级恢复未完成，已保留备份")
		} else {
			diagnostics = append(diagnostics, "已恢复中断的插件升级："+journal.ID)
		}
	}
	return diagnostics
}

func (m *Manager) blockUpgrade(id string) { m.mu.Lock(); m.blockedUpgrades[id] = true; m.mu.Unlock() }
func (m *Manager) upgradeBlocked(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blockedUpgrades[id]
}
func (m *Manager) packageChangesBlocked(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recoveryBlocked || m.blockedUpgrades[id]
}
