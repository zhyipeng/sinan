// Package kernel composes plugin registration, local installation and lifecycle.
package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"sync"
	"time"

	"sinan/internal/llm"
	"sinan/internal/plugin/icon"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/transport"
	"sinan/internal/plugin/uiview"
	"sinan/internal/storage"
	protocol "sinan/packages/plugin-protocol"
)

type PluginInfo struct {
	ActivationEvents       []string                  `json:"activationEvents,omitempty"`
	Icon                   string                    `json:"icon,omitempty"`
	Development            bool                      `json:"development,omitempty"`
	DevelopmentFingerprint string                    `json:"developmentFingerprint,omitempty"`
	NeedsTrust             bool                      `json:"needsTrust,omitempty"`
	Protected              bool                      `json:"protected"`
	UIViews                []protocol.UIView         `json:"uiViews,omitempty"`
	Disabled               bool                      `json:"disabled"`
	Changed                bool                      `json:"changed"`
	Settings               []manifest.Setting        `json:"settings,omitempty"`
	ID                     string                    `json:"id"`
	Name                   string                    `json:"name"`
	Version                string                    `json:"version"`
	Kind                   string                    `json:"kind"`
	State                  string                    `json:"state"`
	Commands               []manifest.Command        `json:"commands"`
	SearchProviders        []manifest.SearchProvider `json:"searchProviders"`
}

type Snapshot struct {
	Plugins     []PluginInfo `json:"plugins"`
	Diagnostics []string     `json:"diagnostics"`
}

type plugin struct {
	development   bool
	nativeTrusted bool
	views         map[*uiview.Session]struct{}
	viewEpoch     uint64
	disabled      bool
	fingerprint   string
	changed       bool
	settings      map[string][]string
	mu            sync.Mutex
	removed       bool
	capabilities  []string
	manifest      manifest.Manifest
	dir           string
	state         string
	process       *transport.Process
}

type Manager struct {
	uiHostCall         func(context.Context, *uiview.Session, protocol.UIRequest) (json.RawMessage, error)
	llm                *llm.Service
	llmTasks           llmTasks
	encryptedDocuments *storage.EncryptedDocuments
	documents          *storage.Documents
	views              map[*uiview.Session]struct{} // Accessed under mutationMu.
	disabled           map[string]bool              // Accessed under mutationMu.
	enablementErr      error
	blockedUpgrades    map[string]bool
	recoveryBlocked    bool
	active             sync.WaitGroup
	lease              *storage.RootLease
	settings           *storage.Settings
	usage              *storage.Usage
	mutationMu         sync.Mutex
	closed             bool
	mu                 sync.Mutex
	root               string
	plugins            map[string]*plugin
	diagnostics        []string
	startupDiagnostics []string
}

func New(root string) (*Manager, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, errors.New("无法创建插件目录")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.New("插件目录无效")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, errors.New("插件目录无效")
	}
	lease, err := storage.AcquireRootLease(filepath.Join(abs, ".sinan-state", "runtime.lock"))
	if err != nil {
		return nil, err
	}
	m := &Manager{blockedUpgrades: map[string]bool{}, lease: lease, root: abs, plugins: make(map[string]*plugin), usage: storage.OpenUsage(filepath.Join(abs, ".sinan-state", "usage.json")), settings: storage.OpenSettings(filepath.Join(abs, ".sinan-state", "settings.json"))}
	m.documents = storage.OpenDocuments(filepath.Join(abs, ".sinan-state", "data"))
	m.encryptedDocuments = storage.OpenEncryptedDocuments(filepath.Join(abs, ".sinan-state", "encrypted-data"), filepath.Join(abs, ".sinan-state", "keys", "plugin-data.key"))
	m.llm = llm.New(m.encryptedDocuments)
	m.llmTasks.items = map[string]*llmTask{}
	m.loadEnablement()
	m.startupDiagnostics = m.recoverUpgrades()
	m.Refresh()
	return m, nil
}

func (m *Manager) Refresh() Snapshot {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Snapshot{Plugins: []PluginInfo{}, Diagnostics: []string{}}
	}
	oldPlugins := m.plugins
	m.mu.Unlock()
	diagnostics := append([]string{}, m.startupDiagnostics...)
	if m.enablementErr != nil {
		diagnostics = append(diagnostics, m.enablementErr.Error())
	}
	if err := m.settings.Error(); err != nil {
		diagnostics = append(diagnostics, err.Error())
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		m.mu.Lock()
		m.diagnostics = append(diagnostics, "无法读取插件目录")
		m.mu.Unlock()
		return m.Snapshot()
	}
	found := map[string]*plugin{}
	for _, entry := range entries {
		id := entry.Name()
		if m.upgradeBlocked(id) {
			continue
		}
		if !manifest.ValidID(id) {
			continue
		}
		dir := filepath.Join(m.root, id)
		var mf manifest.Manifest
		var digest string
		var loadErr error
		if entry.Type()&os.ModeSymlink != 0 {
			dir, loadErr = m.developmentTarget(id)
		} else if !entry.IsDir() {
			loadErr = errors.New("插件目录无效")
		}
		if loadErr == nil {
			mf, digest, loadErr = loadPackage(dir)
			if loadErr == nil && mf.ID != id {
				loadErr = errors.New("插件 ID 不匹配")
			}
		}
		if old := oldPlugins[id]; old != nil {
			old.mu.Lock()
			old.changed = loadErr != nil || digest != old.fingerprint || dir != old.dir
			if old.changed {
				old.revokeViewsLocked()
			}
			old.mu.Unlock()
			found[id] = old
			if loadErr != nil {
				diagnostics = append(diagnostics, "插件文件无效，已保留当前实例："+id)
			}
			continue
		}
		if loadErr != nil {
			diagnostics = append(diagnostics, "已跳过无效插件包："+id)
			continue
		}
		found[id] = m.packageInstance(mf, digest, dir)
	}
	m.mu.Lock()
	m.plugins = found
	m.diagnostics = diagnostics
	m.mu.Unlock()
	for id, p := range oldPlugins {
		if found[id] == nil {
			p.stop(true)
		}
	}
	return m.Snapshot()
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	result := Snapshot{Plugins: []PluginInfo{}, Diagnostics: append([]string{}, m.diagnostics...)}
	entries := make([]*plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		entries = append(entries, p)
	}
	m.mu.Unlock()
	for _, p := range entries {
		p.mu.Lock()
		if p.process != nil && !p.process.Alive() {
			p.process.Stop()
			p.process = nil
			p.state = "failed"
			p.revokeViewsLocked()
		}
		if !p.removed {
			result.Plugins = append(result.Plugins, p.info())
		}
		p.mu.Unlock()
	}
	sort.Slice(result.Plugins, func(i, j int) bool { return result.Plugins[i].ID < result.Plugins[j].ID })
	return result
}

func (p *plugin) info() PluginInfo {
	state := p.state
	if p.disabled {
		state = "disabled"
	}
	developmentFingerprint := ""
	if p.development {
		developmentFingerprint = p.fingerprint
	}
	return PluginInfo{ActivationEvents: p.manifest.ActivationEvents, Icon: icon.Load(p.dir, p.manifest.Icon), Development: p.development, DevelopmentFingerprint: developmentFingerprint, NeedsTrust: p.development && p.manifest.Runtime.Kind == "process" && !p.nativeTrusted, Protected: protectedPlugin(p.manifest.ID), UIViews: p.manifest.Contributes.UIViews, Disabled: p.disabled, Changed: p.changed, Settings: p.manifest.Contributes.Settings, ID: p.manifest.ID, Name: p.manifest.Name, Version: p.manifest.Version, Kind: p.manifest.Runtime.Kind, State: state, Commands: p.manifest.Contributes.Commands, SearchProviders: p.manifest.Contributes.SearchProviders}
}

// Install imports a local directory. Native code requires an explicit trust decision.
func (m *Manager) installLocked(source string, trustNative bool, expected string) (PluginInfo, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return PluginInfo{}, errors.New("运行时已关闭")
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return PluginInfo{}, errors.New("插件目录无效")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return PluginInfo{}, errors.New("插件目录不存在")
	}
	if rel, err := filepath.Rel(abs, m.root); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return PluginInfo{}, errors.New("安装来源不能包含插件安装目录")
	}
	mf, err := manifest.Load(abs)
	if err != nil {
		return PluginInfo{}, err
	}
	if m.packageChangesBlocked(mf.ID) {
		return PluginInfo{}, errors.New("存在未恢复的升级事务，请修复后重启")
	}
	if mf.Runtime.Kind == "process" && !trustNative {
		return PluginInfo{}, errors.New("原生插件需要确认信任后才能安装")
	}
	dest := filepath.Join(m.root, mf.ID)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		return PluginInfo{}, errors.New("插件已存在或目标目录不可用")
	}
	temp, err := os.MkdirTemp(m.root, ".install-")
	if err != nil {
		return PluginInfo{}, errors.New("无法创建安装目录")
	}
	defer os.RemoveAll(temp)
	if err := copyDirectory(abs, temp); err != nil {
		return PluginInfo{}, err
	}
	// Re-read the copied manifest so a concurrent source edit cannot bypass validation.
	copied, err := manifest.Load(temp)
	if err != nil || !reflect.DeepEqual(copied, mf) {
		return PluginInfo{}, errors.New("安装期间插件清单发生变化")
	}
	digest, err := packageFingerprint(temp)
	if err != nil {
		return PluginInfo{}, err
	}
	if expected != "" && digest != expected {
		return PluginInfo{}, errors.New("插件包已变化，请重新检查后再安装")
	}
	if copied.Runtime.Kind == "process" {
		if _, err := manifest.Executable(temp, copied); err != nil {
			return PluginInfo{}, err
		}
	}
	oldApproval := m.nativeApproved(mf.ID)
	if err := os.Rename(temp, dest); err != nil {
		return PluginInfo{}, errors.New("无法完成插件安装")
	}
	if err := m.setNativeApproval(mf.ID, copied.Runtime.Kind == "process"); err != nil {
		_ = os.Rename(dest, temp)
		_ = m.setNativeApproval(mf.ID, oldApproval)
		return PluginInfo{}, errors.New("无法保存插件信任记录，安装未完成")
	}
	p := m.packageInstance(copied, digest, dest)
	info := p.info()
	m.mu.Lock()
	m.plugins[mf.ID] = p
	m.mu.Unlock()
	return info, nil
}

func copyDirectory(source, dest string) error {
	var total int64
	count := 0
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("无法读取插件文件")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return errors.New("插件路径无效")
		}
		if rel == "." {
			return nil
		}
		if count++; count > 2048 {
			return errors.New("插件文件数量超过限制")
		}
		info, err := entry.Info()
		if err != nil || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("插件不允许包含符号链接")
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return errors.New("插件仅允许普通文件")
		}
		in, err := os.Open(path)
		if err != nil {
			return errors.New("无法读取插件文件")
		}
		defer in.Close()
		mode := fs.FileMode(0600)
		if info.Mode().Perm()&0111 != 0 {
			mode = 0700
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return errors.New("无法写入插件文件")
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, (64<<20)-total+1))
		closeErr := out.Close()
		total += n
		if copyErr != nil || closeErr != nil || total > 64<<20 {
			return errors.New("插件复制失败或超过 64 MiB 限制")
		}
		return nil
	})
}

func (m *Manager) lookup(id string) (*plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.plugins[id]
	if m.blockedUpgrades[id] {
		return nil, errors.New("插件升级恢复未完成，请修复后重启")
	}
	if p == nil || m.closed {
		return nil, errors.New("插件不存在或运行时已关闭")
	}
	return p, nil
}

func (m *Manager) current(id string, p *plugin) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.closed && m.plugins[id] == p
}

func (p *plugin) ensureProcess(ctx context.Context, root string) (*transport.Process, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ensureProcessLocked(ctx, root)
}

func (p *plugin) ensureUIProcess(ctx context.Context, root string) (*transport.Process, error) {
	if !p.mu.TryLock() {
		return nil, errors.New("插件正在切换，请稍后重试")
	}
	defer p.mu.Unlock()
	return p.ensureProcessLocked(ctx, root)
}

func (p *plugin) ensureProcessLocked(ctx context.Context, root string) (*transport.Process, error) {
	if p.removed {
		return nil, errors.New("插件已移除")
	}
	if p.disabled {
		return nil, errors.New("插件已禁用")
	}
	if p.process != nil && p.process.Alive() {
		return p.process, nil
	}
	if p.process != nil {
		p.process.Stop()
		p.process = nil
		p.revokeViewsLocked()
	}
	if p.changed {
		return nil, errors.New("插件文件已变化，请先重载")
	}
	approval, err := os.ReadFile(filepath.Join(root, ".trusted", p.manifest.ID))
	approved := err == nil && string(approval) == "trusted-native-v1\n"
	if p.development {
		approved = developmentApproved(root, p.manifest.ID, p.dir)
	}
	if !approved {
		if p.development {
			return nil, errors.New("此开发插件未经信任确认，请在插件管理中确认")
		}
		return nil, errors.New("此原生插件未经信任确认，请重新安装")
	}
	path, err := manifest.Executable(p.dir, p.manifest)
	if err != nil {
		p.state = "failed"
		return nil, err
	}
	p.state = "activating"
	process, err := transport.Start(path, p.dir)
	if err != nil {
		p.state = "failed"
		return nil, err
	}
	var init protocol.InitializeResult
	// Disposable cache stays outside plugin packages so upgrades do not remove it.
	cacheDir := filepath.Join(root, ".sinan-state", "cache", p.manifest.ID)
	if os.MkdirAll(cacheDir, 0700) != nil {
		cacheDir = "" // Cache failure must not prevent plugin activation.
	}
	dataDir := filepath.Join(root, ".sinan-state", "native-data", p.manifest.ID)
	if os.MkdirAll(dataDir, 0700) != nil {
		process.Stop()
		p.state = "failed"
		return nil, errors.New("无法创建插件持久数据目录")
	}
	err = process.Call(ctx, "initialize", protocol.InitializeParams{APIVersion: protocol.APIVersion, PluginID: p.manifest.ID, Settings: p.settings, CacheDir: cacheDir, DataDir: dataDir}, &init)
	if err != nil || init.APIVersion != protocol.APIVersion || init.PluginID != p.manifest.ID {
		process.Stop()
		p.state = "failed"
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("插件握手失败或版本不兼容")
	}
	p.process = process
	p.capabilities = init.Capabilities
	p.state = "running"
	return process, nil
}

func (p *plugin) failed(process *transport.Process) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == process {
		process.Stop()
		p.process = nil
		p.state = "failed"
		p.revokeViewsLocked()
	}
}

func (m *Manager) Execute(ctx context.Context, pluginID, commandID, input string) (result protocol.CommandResult, err error) {
	if err = m.beginOperation(); err != nil {
		return protocol.CommandResult{}, err
	}
	defer m.active.Done()
	defer func() {
		if err == nil {
			m.usage.Record(pluginID, commandID, input)
		}
	}()
	if len(input) > 64<<10 {
		return protocol.CommandResult{}, errors.New("命令输入超过限制")
	}
	if err := ctx.Err(); err != nil {
		return protocol.CommandResult{}, err
	}
	p, err := m.lookup(pluginID)
	if err != nil {
		return protocol.CommandResult{}, err
	}
	p.mu.Lock()
	disabled := p.disabled
	p.mu.Unlock()
	if disabled {
		return protocol.CommandResult{}, errors.New("插件已禁用")
	}
	var command *manifest.Command
	for _, c := range p.manifest.Contributes.Commands {
		if c.ID == commandID {
			command = &c
			break
		}
	}
	if command != nil && (command.View != "" || command.UIView != "") {
		return protocol.CommandResult{}, errors.New("此命令需要在桌面视图中打开")
	}
	if command == nil {
		return protocol.CommandResult{}, errors.New("命令不存在")
	}
	if p.manifest.Runtime.Kind == "declarative" {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.removed || p.disabled {
			return protocol.CommandResult{}, errors.New("插件已移除")
		}
		p.state = "activated"
		return protocol.CommandResult{Text: command.Message}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	process, err := p.ensureProcess(ctx, m.root)
	if err != nil {
		return protocol.CommandResult{}, err
	}
	if err := process.Call(ctx, "command.execute", protocol.CommandParams{CommandID: commandID, Input: input}, &result); err != nil {
		p.failed(process)
		return result, err
	}
	if !m.current(pluginID, p) {
		return protocol.CommandResult{}, errors.New("插件已移除")
	}
	p.mu.Lock()
	current := p.process == process && !p.removed && !p.disabled
	p.mu.Unlock()
	if !current {
		return protocol.CommandResult{}, errors.New("插件已停止")
	}
	return result, nil
}

func (m *Manager) QueryProvider(ctx context.Context, pluginID string, params protocol.SearchParams) (protocol.SearchResult, error) {
	if err := m.beginOperation(); err != nil {
		return protocol.SearchResult{}, err
	}
	defer m.active.Done()
	p, err := m.lookup(pluginID)
	if err != nil {
		return protocol.SearchResult{}, err
	}
	valid := false
	for _, provider := range p.manifest.Contributes.SearchProviders {
		if provider.ID == params.ProviderID {
			valid = true
			break
		}
	}
	if !valid {
		return protocol.SearchResult{}, errors.New("搜索提供者未声明")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	process, err := p.ensureProcess(ctx, m.root)
	if err != nil {
		return protocol.SearchResult{}, err
	}
	p.mu.Lock()
	capable := slices.Contains(p.capabilities, "search.query") && slices.Contains(p.capabilities, "$/cancelRequest")
	p.mu.Unlock()
	if !capable {
		return protocol.SearchResult{}, errors.New("插件未协商搜索与取消能力")
	}
	var result protocol.SearchResult
	err = process.CallSearch(ctx, params, &result)
	if err != nil {
		if !process.Alive() {
			p.failed(process)
		}
		return protocol.SearchResult{}, err
	}
	if !m.current(pluginID, p) {
		return protocol.SearchResult{}, errors.New("插件已移除")
	}
	p.mu.Lock()
	current := p.process == process && !p.removed && !p.disabled
	p.mu.Unlock()
	if !current {
		return protocol.SearchResult{}, errors.New("插件已停止")
	}
	return result, nil
}

func (p *plugin) stop(removed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removed = p.removed || removed
	p.revokeViewsLocked()
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	p.state = "deactivated"
}

func (m *Manager) Stop(id string) error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(id)
	if err != nil {
		return err
	}
	p.stop(false)
	return nil
}

func (m *Manager) Uninstall(id string) error {
	return m.uninstall(id, "")
}

// UninstallChecked removes only the package reviewed by a migration or caller.
// The fingerprint check and removal share the normal mutation lock.
func (m *Manager) UninstallChecked(id, expected string) error {
	if len(expected) != 64 {
		return errors.New("请先核验待卸载插件")
	}
	return m.uninstall(id, expected)
}

func (m *Manager) uninstall(id, expected string) error {
	if protectedPlugin(id) {
		return errors.New("系统插件不允许卸载")
	}
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.packageChangesBlocked(id) {
		return errors.New("存在未恢复的升级事务，请修复后重启")
	}
	m.mu.Lock()
	p := m.plugins[id]
	if p == nil || !manifest.ValidID(id) {
		m.mu.Unlock()
		return errors.New("插件不存在")
	}
	if expected != "" {
		actual, err := packageFingerprint(p.dir)
		if err != nil || p.development || actual != expected || p.fingerprint != expected {
			m.mu.Unlock()
			return errors.New("插件文件已变化，未卸载")
		}
	}
	delete(m.plugins, id)
	m.mu.Unlock()
	p.stop(true)
	var removeErr error
	if p.development {
		link := filepath.Join(m.root, id)
		// Remove a link only: never recurse into the external build directory.
		if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink != 0 {
			removeErr = os.Remove(link)
		} else if !os.IsNotExist(err) {
			removeErr = errors.New("开发链接已被替换，未删除任何目录")
		}
	} else {
		removeErr = os.RemoveAll(p.dir)
	}
	if removeErr != nil {
		return errors.New("无法移除插件文件")
	}
	_ = os.Remove(filepath.Join(m.root, ".trusted", id))
	_ = os.Remove(developmentApprovalPath(m.root, id))
	return nil
}

func (m *Manager) Close() {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	m.closed = true
	m.llmTasks.cancelAll()
	entries := m.plugins
	m.plugins = map[string]*plugin{}
	m.mu.Unlock()
	for _, p := range entries {
		p.stop(true)
	}
	m.views = nil
	m.active.Wait()
	if m.lease != nil {
		m.lease.Close()
		m.lease = nil
	}
}

// UsageBoost is host-owned ranking metadata; plugins cannot supply history weights.
func (m *Manager) UsageBoost(pluginID, commandID, input string) float64 {
	return m.usage.Boost(pluginID, commandID, input)
}
func (m *Manager) ClearUsage() error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return errors.New("运行时已关闭")
	}
	return m.usage.Clear()
}
func (m *Manager) beginOperation() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("运行时已关闭")
	}
	m.active.Add(1)
	return nil
}
