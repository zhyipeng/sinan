package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"sinan/internal/inputmatch"
	"sinan/internal/plugin/icon"
	protocol "sinan/packages/plugin-protocol"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$`)
var commandPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Manifest struct {
	Icon             string        `json:"icon,omitempty"`
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	APIVersion       string        `json:"apiVersion"`
	Runtime          Runtime       `json:"runtime"`
	Contributes      Contributions `json:"contributes"`
	Permissions      []string      `json:"permissions"`
	ActivationEvents []string      `json:"activationEvents"`
}

type Runtime struct {
	Kind  string `json:"kind"`
	Entry string `json:"entry,omitempty"`
}

type Contributions struct {
	Settings        []Setting         `json:"settings,omitempty"`
	Commands        []Command         `json:"commands"`
	SearchProviders []SearchProvider  `json:"searchProviders,omitempty"`
	UIViews         []protocol.UIView `json:"uiViews,omitempty"`
}

type Setting struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type SearchProvider struct {
	ID        string `json:"id"`
	Prefix    string `json:"prefix,omitempty"`
	MinLength int    `json:"minLength,omitempty"`
}

type Command struct {
	View          string                  `json:"view,omitempty"`
	ID            string                  `json:"id"`
	Title         string                  `json:"title"`
	Keywords      []string                `json:"keywords,omitempty"`
	Message       string                  `json:"message,omitempty"`
	Execution     string                  `json:"execution,omitempty"`
	SearchOnly    bool                    `json:"searchOnly,omitempty"`
	UIView        string                  `json:"uiView,omitempty"`
	InputMatchers []protocol.InputMatcher `json:"inputMatchers,omitempty"`
}

func ValidID(id string) bool { return idPattern.MatchString(id) && len(id) <= 128 }

func Load(dir string) (Manifest, error) {
	var m Manifest
	f, err := os.Open(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, errors.New("无法读取 manifest.json")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(b) > 65536 {
		return m, errors.New("manifest 超过大小限制或读取失败")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, errors.New("manifest JSON 无效或包含不支持的字段")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, errors.New("manifest 包含多余内容")
	}
	return m, m.Validate()
}

func (m Manifest) Validate() error {
	if !icon.ValidPath(m.Icon) {
		return errors.New("插件图标必须是包内 SVG、PNG 或 JPEG 相对路径")
	}
	if !ValidID(m.ID) || strings.TrimSpace(m.Name) == "" || !versionPattern.MatchString(m.Version) {
		return errors.New("插件 ID、名称或版本无效")
	}
	if m.APIVersion != protocol.APIVersion {
		return errors.New("插件 API 版本不兼容")
	}
	permissions := map[string]bool{}
	for _, permission := range m.Permissions {
		if m.Runtime.Kind != "process" || (permission != "applications.read" && permission != "applications.launch") || permissions[permission] {
			return errors.New("权限用途无效或重复；当前仅支持可信原生插件的应用读取与启动声明")
		}
		permissions[permission] = true
	}
	if len(m.Contributes.Commands) == 0 {
		return errors.New("插件至少需要提供一个命令")
	}
	if len(m.Contributes.UIViews) > 4 {
		return errors.New("UI 视图数量超过限制")
	}
	views := map[string]bool{}
	for _, view := range m.Contributes.UIViews {
		if view.Title != "" && (strings.TrimSpace(view.Title) == "" || utf8.RuneCountInString(view.Title) > 64 || strings.ContainsFunc(view.Title, unicode.IsControl)) {
			return errors.New("UI 视图标题无效")
		}

		if !ValidViewID(view.ID) || views[view.ID] || !ValidUIEntry(view.Entry) || !protocol.SupportedUIBridge(view.BridgeVersion) || !validUICapabilities(view, m) {
			return errors.New("UI 视图声明无效、版本不兼容或请求了不支持的权限")
		}
		views[view.ID] = true
	}
	seen := map[string]bool{}
	matcherCount := 0
	for _, c := range m.Contributes.Commands {
		matcherCount += len(c.InputMatchers)
		if len(c.InputMatchers) > inputmatch.MaxMatchersPerCommand || matcherCount > inputmatch.MaxMatchersPerPlugin {
			return errors.New("输入匹配规则数量超过限制")
		}
		if len(c.InputMatchers) > 0 {
			if c.View != "" || c.SearchOnly {
				return errors.New("输入匹配不支持可信组件视图或 searchOnly 命令")
			}
			for _, view := range m.Contributes.UIViews {
				if view.ID == c.UIView && view.BridgeVersion != protocol.UIRequestBridgeVersion {
					return errors.New("输入匹配需要 UI 桥 2")
				}
			}
			for _, rule := range c.InputMatchers {
				if rule.Kind == "image" && c.UIView == "" {
					return errors.New("图片匹配仅支持 HTML 视图命令")
				}
				if err := inputmatch.Validate(rule); err != nil {
					return err
				}
			}
		}
		if c.Execution != "" && c.Execution != "direct" {
			return errors.New("命令执行模式无效")
		}
		if !commandPattern.MatchString(c.ID) || strings.TrimSpace(c.Title) == "" || seen[c.ID] {
			return errors.New("命令 ID、标题无效或重复")
		}
		seen[c.ID] = true
		if c.UIView != "" && (!views[c.UIView] || c.View != "" || c.Message != "" || c.Execution != "") {
			return errors.New("独立视图命令必须引用已声明视图，且不能混用其他执行方式")
		}
		if c.View != "" && (views[c.View] || !commandPattern.MatchString(c.View) || c.Message != "" || c.Execution != "" || m.Runtime.Kind != "declarative") {
			return errors.New("视图命令声明无效")
		}
		if m.Runtime.Kind == "declarative" && c.Message == "" && c.View == "" && c.UIView == "" {
			return errors.New("声明式命令需要 message")
		}
		if m.Runtime.Kind == "process" && c.Message != "" {
			return errors.New("进程命令不支持 message")
		}
	}
	for _, event := range m.ActivationEvents {
		if event == "onSearch" && len(m.Contributes.SearchProviders) > 0 {
			continue
		}
		if !strings.HasPrefix(event, "onCommand:") || !seen[strings.TrimPrefix(event, "onCommand:")] {
			return errors.New("不支持的激活事件或未声明对应贡献")
		}
	}
	if len(m.Contributes.Settings) > 16 {
		return errors.New("设置项数量超过限制")
	}
	settingIDs := map[string]bool{}
	for _, setting := range m.Contributes.Settings {
		if !commandPattern.MatchString(setting.ID) || settingIDs[setting.ID] || strings.TrimSpace(setting.Title) == "" || len(setting.Title) > 128 || setting.Type != "directory-list" {
			return errors.New("设置项声明无效或重复")
		}
		settingIDs[setting.ID] = true
	}
	providers := map[string]bool{}
	if len(m.Contributes.SearchProviders) > 8 {
		return errors.New("搜索提供者数量超过限制")
	}
	for _, p := range m.Contributes.SearchProviders {
		if m.Runtime.Kind != "process" || !commandPattern.MatchString(p.ID) || providers[p.ID] || p.MinLength < 0 || p.MinLength > 256 || len(p.Prefix) > 64 {
			return errors.New("搜索提供者声明无效或重复")
		}
		providers[p.ID] = true
	}
	switch m.Runtime.Kind {
	case "declarative":
		if m.Runtime.Entry != "" {
			return errors.New("声明式插件不支持运行入口")
		}
	case "process":
		if m.Runtime.Entry == "" || !filepath.IsLocal(m.Runtime.Entry) || strings.ContainsAny(m.Runtime.Entry, `\:$`) {
			return errors.New("进程入口必须是插件内的相对路径")
		}
	default:
		return fmt.Errorf("不支持的插件运行模式")
	}
	return nil
}

// Executable rejects symlinks as well as lexical escapes from the plugin root.
func Executable(dir string, m Manifest) (string, error) {
	path := filepath.Join(dir, m.Runtime.Entry)
	resolved, err := filepath.EvalSymlinks(path)
	abs, absErr := filepath.Abs(path)
	if err != nil || absErr != nil || resolved != abs {
		return "", errors.New("插件入口不存在或包含符号链接")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("插件入口不是可执行文件")
	}
	return path, nil
}

func validUICapabilities(view protocol.UIView, mf Manifest) bool {
	if view.BridgeVersion == protocol.UIBridgeVersion {
		return len(view.Commands) == 0 && len(view.Permissions) == 0
	}
	if len(view.Permissions) > 1 || (len(view.Permissions) == 1 && view.Permissions[0] != "storage") || len(view.Commands) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range view.Commands {
		if seen[id] || mf.Runtime.Kind != "process" {
			return false
		}
		seen[id] = true
		found := false
		for _, cmd := range mf.Contributes.Commands {
			if cmd.ID == id && cmd.View == "" && cmd.UIView == "" {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
