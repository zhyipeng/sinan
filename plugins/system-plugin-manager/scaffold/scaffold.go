// Package scaffold owns the bundled plugin manager's project templates.
// It writes source files only; it never builds, installs or executes a plugin.
package scaffold

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
)

//go:embed templates/*
var templates embed.FS

const TemplateVersion = "4"

type Options struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Template  string `json:"template"`
	Directory string `json:"directory"`
}

type Result struct {
	ID               string `json:"id"`
	Directory        string `json:"directory"`
	PackageDirectory string `json:"packageDirectory"`
}

// Create places a new project under Directory/ID. Existing paths are never reused.
func Create(options Options) (Result, error) {
	files, err := projectFiles(options)
	if err != nil {
		return Result{}, err
	}
	if !filepath.IsAbs(options.Directory) {
		return Result{}, errors.New("请选择父目录的绝对路径")
	}
	parent, err := os.OpenRoot(options.Directory)
	if err != nil {
		return Result{}, errors.New("无法打开父目录")
	}
	defer parent.Close()
	if err = parent.Mkdir(options.ID, 0755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Result{}, errors.New("插件目录已存在，请修改插件 ID 或选择其他父目录")
		}
		return Result{}, errors.New("无法创建插件目录，请检查目录权限")
	}
	project, err := parent.OpenRoot(options.ID)
	if err != nil {
		return Result{}, errors.New("无法打开新建目录，请检查并清理未完成的项目")
	}
	defer project.Close()
	for name, data := range files {
		if err = project.MkdirAll(filepath.Dir(name), 0755); err == nil {
			var file *os.File
			file, err = project.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err == nil {
				_, err = file.Write(data)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if err != nil {
			return Result{}, errors.New("模板写入失败，请检查并清理未完成的项目；未覆盖已有文件")
		}
	}
	directory := filepath.Join(options.Directory, options.ID)
	return Result{ID: options.ID, Directory: directory, PackageDirectory: filepath.Join(directory, "dist", "plugin")}, nil
}

func projectFiles(o Options) (map[string][]byte, error) {
	if strings.ContainsFunc(o.Name, unicode.IsControl) {
		return nil, errors.New("插件名不能包含控制字符")
	}
	o.Name = strings.TrimSpace(o.Name)
	if o.Name == "" || !utf8.ValidString(o.Name) || utf8.RuneCountInString(o.Name) > 64 {
		return nil, errors.New("插件名需为 1–64 个字符，且不能包含控制字符")
	}
	if !manifest.ValidID(o.ID) || strings.HasPrefix(o.ID, "system.") {
		return nil, errors.New("插件 ID 需为小写分段标识（如 local.my-plugin），不能使用 system 命名空间")
	}
	if o.Template != "go-html" && o.Template != "html" {
		return nil, errors.New("不支持的插件模板")
	}
	files := map[string][]byte{}
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	targets := map[string]string{
		"index.html": "frontend/index.html", "style.css": "frontend/src/style.css",
		"main.jsx":          "frontend/src/main.jsx",
		"package.json.tmpl": "package.json", "package-lock.json.tmpl": "package-lock.json",
		"html-app.jsx": "frontend/src/App.jsx", "go-app.jsx": "frontend/src/App.jsx",
		"go-main.go.tmpl": "cmd/main.go", "go-mod.tmpl": "go.mod",
		"build.mjs": "scripts/build.mjs", "README.md.tmpl": "README.md",
		"gitignore.tmpl": ".gitignore",
		"package.py":     "scripts/package.py", "release.mjs": "scripts/release.mjs",
		"registry-pr.mjs": "scripts/registry-pr.mjs", "release.yml.tmpl": ".github/workflows/release.yml",
		"AGENTS.md.tmpl": "AGENTS.md",
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "go-") && o.Template != "go-html" {
			continue
		}
		data, err := templates.ReadFile("templates/" + name)
		if err != nil {
			return nil, err
		}
		if name == "html-app.jsx" && o.Template == "go-html" {
			continue
		}
		replace := strings.NewReplacer("__PLUGIN_ID__", o.ID, "__TEMPLATE__", o.Template)
		target, ok := targets[name]
		if !ok {
			return nil, fmt.Errorf("未知模板文件：%s", name)
		}
		data = []byte(replace.Replace(string(data)))
		if strings.HasSuffix(target, ".go") {
			data, err = format.Source(data)
			if err != nil {
				return nil, err
			}
		}
		files[target] = data
	}
	mf := manifest.Manifest{ID: o.ID, Name: o.Name, Version: "0.1.0", APIVersion: protocol.APIVersion, Permissions: []string{}, ActivationEvents: []string{"onCommand:open"}}
	mf.Runtime.Kind = "declarative"
	mf.Contributes.Commands = []manifest.Command{{ID: "open", Title: o.Name, UIView: "main"}}
	mf.Contributes.UIViews = []protocol.UIView{{ID: "main", Entry: "index.html", BridgeVersion: "2", Permissions: []string{"storage"}}}
	if o.Template == "go-html" {
		mf.Runtime.Kind = "process"
		mf.Runtime.Entry = "backend/plugin"
		mf.Contributes.Commands = append(mf.Contributes.Commands, manifest.Command{ID: "echo", Title: "回声", SearchOnly: true})
		mf.Contributes.UIViews[0].Commands = []string{"echo"}
		mf.ActivationEvents = append(mf.ActivationEvents, "onCommand:echo")
		files["third_party/sinan/go.mod"] = []byte("module sinan\n\ngo 1.26.0\n")
		files["third_party/sinan/packages/plugin-protocol/protocol.go"] = []byte(protocol.ScaffoldSource())
		for name, data := range sdk.ScaffoldSources() {
			files["third_party/sinan/packages/plugin-sdk/"+name] = data
		}
	}
	files["frontend/src/bridge.js"] = sdk.ScaffoldSources()["ui/index.js"]
	if err := mf.Validate(); err != nil {
		return nil, fmt.Errorf("模板清单无效：%w", err)
	}
	files["manifest.json"], _ = json.MarshalIndent(mf, "", "  ")
	files["manifest.json"] = append(files["manifest.json"], '\n')
	provenance := struct {
		TemplateVersion string            `json:"templateVersion"`
		Template        string            `json:"template"`
		Files           map[string]string `json:"sdkFiles"`
	}{TemplateVersion: TemplateVersion, Template: o.Template, Files: map[string]string{}}
	for name, data := range files {
		if strings.HasPrefix(name, "third_party/") || name == "frontend/src/bridge.js" {
			sum := sha256.Sum256(data)
			provenance.Files[name] = hex.EncodeToString(sum[:])
		}
	}
	files["sinan-template.json"], _ = json.MarshalIndent(provenance, "", "  ")
	return files, nil
}
