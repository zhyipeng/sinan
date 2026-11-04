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
	"sort"
	"sync"
	"time"

	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/transport"
	protocol "sinan/packages/plugin-protocol"
)

type PluginInfo struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Version  string             `json:"version"`
	Kind     string             `json:"kind"`
	State    string             `json:"state"`
	Commands []manifest.Command `json:"commands"`
}

type Snapshot struct {
	Plugins     []PluginInfo `json:"plugins"`
	Diagnostics []string     `json:"diagnostics"`
}

type plugin struct {
	manifest manifest.Manifest
	dir      string
	state    string
	process  *transport.Process
}

type Manager struct {
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
	m := &Manager{root: abs, plugins: make(map[string]*plugin)}
	m.Refresh()
	return m, nil
}

func (m *Manager) Refresh() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.diagnostics = []string{}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		m.diagnostics = append(m.diagnostics, "无法读取插件目录")
		return m.snapshot()
	}
	found := map[string]bool{}
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
			if old.process != nil {
				old.process.Stop()
			}
		}
		m.plugins[mf.ID] = &plugin{manifest: mf, dir: dir, state: "installed"}
	}
	for id, p := range m.plugins {
		if !found[id] {
			if p.process != nil {
				p.process.Stop()
			}
			delete(m.plugins, id)
		}
	}
	return m.snapshot()
}

func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot() }

func (m *Manager) snapshot() Snapshot {
	result := Snapshot{Plugins: []PluginInfo{}, Diagnostics: append([]string{}, m.diagnostics...)}
	for _, p := range m.plugins {
		if p.process != nil && !p.process.Alive() {
			p.process.Stop()
			p.process = nil
			p.state = "failed"
		}
		result.Plugins = append(result.Plugins, PluginInfo{ID: p.manifest.ID, Name: p.manifest.Name, Version: p.manifest.Version, Kind: p.manifest.Runtime.Kind, State: p.state, Commands: p.manifest.Contributes.Commands})
	}
	sort.Slice(result.Plugins, func(i, j int) bool { return result.Plugins[i].ID < result.Plugins[j].ID })
	return result
}

// Install imports a local directory. Native code requires an explicit trust decision.
func (m *Manager) Install(source string, trustNative bool) (PluginInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.plugins[mf.ID] = &plugin{manifest: copied, dir: dest, state: "installed"}
	return PluginInfo{ID: copied.ID, Name: copied.Name, Version: copied.Version, Kind: copied.Runtime.Kind, State: "installed", Commands: copied.Contributes.Commands}, nil
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

func (m *Manager) Execute(ctx context.Context, pluginID, commandID, input string) (protocol.CommandResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(input) > 64<<10 {
		return protocol.CommandResult{}, errors.New("命令输入超过 64 KiB 限制")
	}
	p := m.plugins[pluginID]
	if p == nil {
		return protocol.CommandResult{}, errors.New("插件不存在")
	}
	var command *manifest.Command
	for _, c := range p.manifest.Contributes.Commands {
		if c.ID == commandID {
			command = &c
			break
		}
	}
	if command == nil {
		return protocol.CommandResult{}, errors.New("命令不存在")
	}
	if p.manifest.Runtime.Kind == "declarative" {
		p.state = "activated"
		return protocol.CommandResult{Text: command.Message}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p.process == nil || !p.process.Alive() {
		if p.process != nil {
			p.process.Stop()
			p.process = nil
		}
		approval, err := os.ReadFile(filepath.Join(m.root, ".trusted", pluginID))
		if err != nil || string(approval) != "trusted-native-v1\n" {
			return protocol.CommandResult{}, errors.New("此原生插件未经信任确认，请通过安装入口重新安装")
		}
		path, err := manifest.Executable(p.dir, p.manifest)
		if err != nil {
			p.state = "failed"
			return protocol.CommandResult{}, err
		}
		p.state = "activating"
		p.process, err = transport.Start(path, p.dir)
		if err != nil {
			p.state = "failed"
			return protocol.CommandResult{}, err
		}
		var init protocol.InitializeResult
		err = p.process.Call(ctx, "initialize", protocol.InitializeParams{APIVersion: protocol.APIVersion, PluginID: pluginID}, &init)
		if err != nil || init.APIVersion != protocol.APIVersion || init.PluginID != pluginID {
			p.process.Stop()
			p.process = nil
			p.state = "failed"
			return protocol.CommandResult{}, errors.New("插件握手失败或版本不兼容")
		}
	}
	p.state = "running"
	var result protocol.CommandResult
	if err := p.process.Call(ctx, "command.execute", protocol.CommandParams{CommandID: commandID, Input: input}, &result); err != nil {
		p.process.Stop()
		p.process = nil
		p.state = "failed"
		return protocol.CommandResult{}, err
	}
	return result, nil
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.plugins[id]
	if p == nil {
		return errors.New("插件不存在")
	}
	if p.process != nil {
		p.process.Stop()
		p.process = nil
	}
	p.state = "deactivated"
	return nil
}

func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.plugins[id]
	if p == nil || !manifest.ValidID(id) {
		return errors.New("插件不存在")
	}
	if p.process != nil {
		p.process.Stop()
		p.process = nil
		p.state = "deactivated"
	}
	if err := os.RemoveAll(p.dir); err != nil {
		return errors.New("无法移除插件文件")
	}
	_ = os.Remove(filepath.Join(m.root, ".trusted", id))
	delete(m.plugins, id)
	return nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.plugins {
		if p.process != nil {
			p.process.Stop()
			p.process = nil
			p.state = "deactivated"
		}
	}
}
