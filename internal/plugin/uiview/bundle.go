// Package uiview builds bounded, immutable UI resource snapshots. It does not
// load installed HTML into the trusted Shell or grant host API permissions.
package uiview

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"sinan/internal/plugin/manifest"
)

const (
	MaxFiles       = 128
	MaxFileBytes   = 512 << 10
	MaxBundleBytes = 2 << 20
)

const CSP = "default-src 'none'; script-src sinan-plugin:; style-src sinan-plugin:; img-src sinan-plugin:; connect-src 'none'; frame-src 'none'; child-src 'none'; worker-src 'none'; object-src 'none'; media-src 'none'; font-src 'none'; form-action 'none'; base-uri 'none'"

var localPath = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)
var mediaTypes = map[string]string{
	".html": "text/html", ".js": "application/javascript", ".css": "text/css",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp",
}

type Resource struct {
	MIME string `json:"mime"`
	Data []byte `json:"data"`
}
type Bundle struct {
	owner     string
	entry     string
	resources map[string]Resource
}

func validPath(name string) bool {
	return name != "." && len(name) <= 256 && fs.ValidPath(name) && path.Clean(name) == name && localPath.MatchString(name) && strings.Count(name, "/") <= 8
}

// LoadDirectory confines traversal with os.Root and rejects symlink entries.
// The caller must pass the intended UI directory, never the whole plugin root.
func LoadDirectory(ctx context.Context, directory, owner, entry string) (*Bundle, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, errors.New("UI 资源根目录无效")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, errors.New("无法打开 UI 资源目录")
	}
	defer root.Close()
	return LoadFS(ctx, root.FS(), owner, entry)
}

func LoadFS(ctx context.Context, source fs.FS, owner, entry string) (*Bundle, error) {
	if !manifest.ValidID(owner) || !validPath(entry) || path.Ext(entry) != ".html" {
		return nil, errors.New("UI 归属或入口无效")
	}
	bundle := &Bundle{owner: owner, entry: entry, resources: map[string]Resource{}}
	var count, total int
	err := fs.WalkDir(source, ".", func(name string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return errors.New("UI 资源无法读取")
		}
		if name == "." {
			return nil
		}
		count++
		if count > MaxFiles || !validPath(name) || d.Type()&fs.ModeSymlink != 0 {
			return errors.New("UI 资源路径、类型或数量无效")
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
			return errors.New("UI 资源类型或大小无效")
		}
		mime := mediaTypes[path.Ext(name)]
		if mime == "" {
			return errors.New("UI 资源类型不支持")
		}
		f, err := source.Open(name)
		if err != nil {
			return errors.New("UI 资源无法读取")
		}
		data, readErr := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
		closeErr := f.Close()
		total += len(data)
		if readErr != nil || closeErr != nil || len(data) > MaxFileBytes || total > MaxBundleBytes {
			return errors.New("UI 资源读取失败或超过预算")
		}
		bundle.resources[bundle.url(name)] = Resource{MIME: mime, Data: data}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, ok := bundle.resources[bundle.EntryURL()]; !ok {
		return nil, errors.New("UI 入口不存在")
	}
	return bundle, nil
}

func (b *Bundle) url(name string) string { return "sinan-plugin://" + b.owner + "/" + name }
func (b *Bundle) EntryURL() string       { return b.url(b.entry) }

// Resource rejects noncanonical URLs as well as cross-plugin ownership. The
// renderer gets bytes from this snapshot, never filesystem paths or handles.
func (b *Bundle) Resource(rawURL string) (Resource, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "sinan-plugin" || u.Host != b.owner || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || !validPath(strings.TrimPrefix(u.Path, "/")) {
		return Resource{}, false
	}
	resource, ok := b.resources[rawURL]
	if !ok {
		return Resource{}, false
	}
	resource.Data = append([]byte(nil), resource.Data...)
	return resource, true
}
