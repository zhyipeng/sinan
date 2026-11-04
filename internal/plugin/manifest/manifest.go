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

	protocol "sinan/packages/plugin-protocol"
)

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$`)
var commandPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Manifest struct {
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
	Commands        []Command        `json:"commands"`
	SearchProviders []SearchProvider `json:"searchProviders,omitempty"`
}

type SearchProvider struct {
	ID        string `json:"id"`
	Prefix    string `json:"prefix,omitempty"`
	MinLength int    `json:"minLength,omitempty"`
}

type Command struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Keywords []string `json:"keywords,omitempty"`
	Message  string   `json:"message,omitempty"`
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
	if !ValidID(m.ID) || strings.TrimSpace(m.Name) == "" || !versionPattern.MatchString(m.Version) {
		return errors.New("插件 ID、名称或版本无效")
	}
	if m.APIVersion != protocol.APIVersion {
		return errors.New("插件 API 版本不兼容")
	}
	if len(m.Permissions) != 0 {
		return errors.New("当前原型尚不支持系统权限")
	}
	if len(m.Contributes.Commands) == 0 {
		return errors.New("插件至少需要提供一个命令")
	}
	seen := map[string]bool{}
	for _, c := range m.Contributes.Commands {
		if !commandPattern.MatchString(c.ID) || strings.TrimSpace(c.Title) == "" || seen[c.ID] {
			return errors.New("命令 ID、标题无效或重复")
		}
		seen[c.ID] = true
		if m.Runtime.Kind == "declarative" && c.Message == "" {
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
