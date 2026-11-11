package kernel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) developmentTarget(id string) (string, error) {
	if strings.HasPrefix(id, "system.") {
		return "", errors.New("内置插件不支持开发链接")
	}
	link := filepath.Join(m.root, id)
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", errors.New("开发插件链接不存在")
	}
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return "", errors.New("开发链接目标不可用")
	}
	if rel, err := filepath.Rel(m.root, target); err != nil || rel == "." || filepath.IsLocal(rel) {
		return "", errors.New("开发链接目标必须位于插件安装目录之外")
	}
	return target, nil
}

func developmentApprovalPath(root, id string) string {
	return filepath.Join(root, ".sinan-state", "development-trust", id+".json")
}
func developmentApproved(root, id, target string) bool {
	data, err := os.ReadFile(developmentApprovalPath(root, id))
	var record struct {
		Target string `json:"target"`
	}
	return err == nil && json.Unmarshal(data, &record) == nil && record.Target == target
}

// TrustDevelopmentPlugin binds explicit native trust to the reviewed content and
// canonical development path. It does not execute the plugin or approve new paths.
func (m *Manager) TrustDevelopmentPlugin(id, expected string) (PluginInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	p, err := m.lookup(id)
	if err != nil {
		return PluginInfo{}, err
	}
	if !p.development || p.manifest.Runtime.Kind != "process" {
		return PluginInfo{}, errors.New("此插件不需要开发信任确认")
	}
	target, err := m.developmentTarget(id)
	if err != nil || target != p.dir {
		return PluginInfo{}, errors.New("开发链接已变化，请重新登记")
	}
	_, digest, err := loadPackage(target)
	if err != nil || expected != p.fingerprint || digest != expected {
		p.markUIChanged()
		return PluginInfo{}, errors.New("开发产物已变化，请刷新并重载后重新确认")
	}
	data, _ := json.Marshal(struct {
		Target string `json:"target"`
	}{target})
	dest := developmentApprovalPath(m.root, id)
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return PluginInfo{}, err
	}
	// Atomic replacement also leaves the previous approval intact on write failure.
	temp, err := os.CreateTemp(filepath.Dir(dest), ".approval-")
	if err != nil {
		return PluginInfo{}, err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return PluginInfo{}, err
	}
	if err = temp.Close(); err != nil {
		return PluginInfo{}, err
	}
	if err = os.Rename(temp.Name(), dest); err != nil {
		return PluginInfo{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nativeTrusted = true
	return p.info(), nil
}
