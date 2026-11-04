// Package kernel composes plugin registration, local installation and lifecycle.
package kernel

import (
	"context"
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

	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/transport"
	"sinan/internal/storage"
	protocol "sinan/packages/plugin-protocol"
)

type PluginInfo struct {
	Settings        []manifest.Setting        `json:"settings,omitempty"`
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Version         string                    `json:"version"`
	Kind            string                    `json:"kind"`
	State           string                    `json:"state"`
	Commands        []manifest.Command        `json:"commands"`
	SearchProviders []manifest.SearchProvider `json:"searchProviders"`
}

type Snapshot struct {
	Plugins     []PluginInfo `json:"plugins"`
	Diagnostics []string     `json:"diagnostics"`
}

type plugin struct {
	settings     map[string][]string
	mu           sync.Mutex
	removed      bool
	capabilities []string
	manifest     manifest.Manifest
	dir          string
	state        string
	process      *transport.Process
}

type Manager struct {
	settings    *storage.Settings
	usage       *storage.Usage
	mutationMu  sync.Mutex
	closed      bool
	mu          sync.Mutex
	root        string
	plugins     map[string]*plugin
	diagnostics []string
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
	m := &Manager{root: abs, plugins: make(map[string]*plugin), usage: storage.OpenUsage(filepath.Join(abs, ".sinan-state", "usage.json")), settings: storage.OpenSettings(filepath.Join(abs, ".sinan-state", "settings.json"))}
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
	m.diagnostics = []string{}
	if err := m.settings.Error(); err != nil {
		m.diagnostics = append(m.diagnostics, err.Error())
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		m.diagnostics = append(m.diagnostics, "无法读取插件目录")
		m.mu.Unlock()
		return m.Snapshot()
	}
	found := map[string]bool{}
	retired := []*plugin{}
	for _, entry := range entries {
		if !manifest.ValidID(entry.Name()) {
			continue
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			m.diagnostics = append(m.diagnostics, "已跳过非目录插件入口")
			continue
		}
		dir := filepath.Join(m.root, entry.Name())
		mf, err := manifest.Load(dir)
		if err != nil || mf.ID != entry.Name() {
			m.diagnostics = append(m.diagnostics, "已跳过无效插件清单")
			continue
		}
		found[mf.ID] = true
		if old := m.plugins[mf.ID]; old != nil {
			if reflect.DeepEqual(old.manifest, mf) {
				continue
			}
			retired = append(retired, old)
		}
		m.plugins[mf.ID] = &plugin{manifest: mf, dir: dir, state: "installed", settings: settingsValues(mf, m.settings.Get(mf.ID).Values)}
	}
	for id, p := range m.plugins {
		if !found[id] {
			retired = append(retired, p)
			delete(m.plugins, id)
		}
	}
	m.mu.Unlock()
	for _, p := range retired {
		p.stop(true)
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
	return PluginInfo{Settings: p.manifest.Contributes.Settings, ID: p.manifest.ID, Name: p.manifest.Name, Version: p.manifest.Version, Kind: p.manifest.Runtime.Kind, State: p.state, Commands: p.manifest.Contributes.Commands, SearchProviders: p.manifest.Contributes.SearchProviders}
}

// Install imports a local directory. Native code requires an explicit trust decision.
func (m *Manager) Install(source string, trustNative bool) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
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
	if err != nil || copied.ID != mf.ID || copied.Runtime.Kind != mf.Runtime.Kind {
		return PluginInfo{}, errors.New("安装期间插件清单发生变化")
	}
	if copied.Runtime.Kind == "process" {
		if _, err := manifest.Executable(temp, copied); err != nil {
			return PluginInfo{}, err
		}
		// Approval is host-owned metadata, never inferred from the manifest.
		if err := os.MkdirAll(filepath.Join(m.root, ".trusted"), 0700); err != nil {
			return PluginInfo{}, errors.New("无法保存插件信任记录")
		}
		if err := os.WriteFile(filepath.Join(m.root, ".trusted", mf.ID), []byte("trusted-native-v1\n"), 0600); err != nil {
			return PluginInfo{}, errors.New("无法保存插件信任记录")
		}
	}
	if err := os.Rename(temp, dest); err != nil {
		return PluginInfo{}, errors.New("无法完成插件安装")
	}
	p := &plugin{manifest: copied, dir: dest, state: "installed", settings: settingsValues(copied, m.settings.Get(copied.ID).Values)}
	m.mu.Lock()
	m.plugins[mf.ID] = p
	m.mu.Unlock()
	return p.info(), nil
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
	if p.removed {
		return nil, errors.New("插件已移除")
	}
	if p.process != nil && p.process.Alive() {
		return p.process, nil
	}
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	approval, err := os.ReadFile(filepath.Join(root, ".trusted", p.manifest.ID))
	if err != nil || string(approval) != "trusted-native-v1\n" {
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
	err = process.Call(ctx, "initialize", protocol.InitializeParams{APIVersion: protocol.APIVersion, PluginID: p.manifest.ID, Settings: p.settings}, &init)
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
	}
}

func (m *Manager) Execute(ctx context.Context, pluginID, commandID, input string) (result protocol.CommandResult, err error) {
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
	var command *manifest.Command
	for _, c := range p.manifest.Contributes.Commands {
		if c.ID == commandID {
			command = &c
			break
		}
	}
	if command != nil && command.View != "" {
		return protocol.CommandResult{}, errors.New("此命令需要在桌面视图中打开")
	}
	if command == nil {
		return protocol.CommandResult{}, errors.New("命令不存在")
	}
	if p.manifest.Runtime.Kind == "declarative" {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.removed {
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
	current := p.process == process && !p.removed
	p.mu.Unlock()
	if !current {
		return protocol.CommandResult{}, errors.New("插件已停止")
	}
	return result, nil
}

func (m *Manager) QueryProvider(ctx context.Context, pluginID string, params protocol.SearchParams) (protocol.SearchResult, error) {
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
	current := p.process == process && !p.removed
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
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	p.state = "deactivated"
}

func (m *Manager) Stop(id string) error {
	p, err := m.lookup(id)
	if err != nil {
		return err
	}
	p.stop(false)
	return nil
}

func (m *Manager) Uninstall(id string) error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	p := m.plugins[id]
	if p == nil || !manifest.ValidID(id) {
		m.mu.Unlock()
		return errors.New("插件不存在")
	}
	delete(m.plugins, id)
	m.mu.Unlock()
	p.stop(true)
	if err := os.RemoveAll(p.dir); err != nil {
		return errors.New("无法移除插件文件")
	}
	_ = os.Remove(filepath.Join(m.root, ".trusted", id))
	return nil
}

func (m *Manager) Close() {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	m.closed = true
	entries := m.plugins
	m.plugins = map[string]*plugin{}
	m.mu.Unlock()
	for _, p := range entries {
		p.stop(true)
	}
}

// UsageBoost is host-owned ranking metadata; plugins cannot supply history weights.
func (m *Manager) UsageBoost(pluginID, commandID, input string) float64 {
	return m.usage.Boost(pluginID, commandID, input)
}
func (m *Manager) ClearUsage() error { return m.usage.Clear() }
