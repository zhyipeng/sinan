// Package distribution provides bounded GitHub asset download and ZIP staging.
package distribution

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*/[A-Za-z0-9_-][A-Za-z0-9_.-]*$`)

type Artifact struct {
	Repository string
	ID         int64
	Size       int64
	SHA256     string
}
type Fetcher struct{ Client *http.Client }

func allowedURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return true
	}
	return false
}
func New() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !allowedURL(req.URL) {
			return errors.New("下载重定向被拒绝")
		}
		return nil
	}}}
}
func (f *Fetcher) Stage(ctx context.Context, cacheDir string, a Artifact) (directory string, cleanup func(), err error) {
	if a.ID <= 0 || a.Size <= 0 || a.Size > 64<<20 || len(a.SHA256) != 64 || strings.Trim(a.SHA256, "0123456789abcdef") != "" || !repositoryPattern.MatchString(a.Repository) {
		return "", nil, errors.New("发行资产参数无效")
	}
	if err = os.MkdirAll(cacheDir, 0700); err != nil {
		return "", nil, errors.New("无法创建下载缓存")
	}
	work, err := os.MkdirTemp(cacheDir, "package-")
	if err != nil {
		return "", nil, errors.New("无法创建下载目录")
	}
	removeWork := func() { _ = os.RemoveAll(work) }
	cleanup = removeWork
	success := false
	defer func() {
		if !success {
			removeWork()
		}
	}()
	address := fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", a.Repository, a.ID)
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return "", nil, errors.New("下载请求无效")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "Sinan-Plugin-Store")
	response, err := f.Client.Do(req)
	if err != nil {
		return "", nil, errors.New("插件包下载失败")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.ContentLength > a.Size {
		return "", nil, errors.New("插件包不可用或大小不符合目录记录")
	}
	archive := filepath.Join(work, "package.zip")
	file, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, errors.New("无法写入下载缓存")
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, a.Size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != a.Size || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return "", nil, errors.New("插件包大小或 SHA-256 校验失败")
	}
	directory = filepath.Join(work, "plugin")
	if err = os.Mkdir(directory, 0700); err != nil {
		return "", nil, err
	}
	if err = extract(archive, directory); err != nil {
		return "", nil, err
	}
	_ = os.Remove(archive)
	success = true
	return directory, cleanup, nil
}
func extract(archive, directory string) error {
	zipped, err := zip.OpenReader(archive)
	if err != nil {
		return errors.New("插件 ZIP 无效")
	}
	defer zipped.Close()
	if len(zipped.File) == 0 || len(zipped.File) > 2048 {
		return errors.New("插件 ZIP 条目数超过限制")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := map[string]bool{}
	var total uint64
	manifestFound := false
	for _, entry := range zipped.File {
		name := strings.TrimSuffix(entry.Name, "/")
		mode := entry.Mode()
		key := strings.ToLower(name)
		if !utf8.ValidString(name) || len(name) > 256 || name == "" || name == "." || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || strings.Count(name, "/") > 8 || seen[key] || mode&os.ModeSymlink != 0 || mode.Type() != 0 && !mode.IsDir() {
			return errors.New("插件 ZIP 包含不安全路径、重复条目或特殊文件")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." || part == "." {
				return errors.New("插件 ZIP 路径越界")
			}
		}
		seen[key] = true
		if mode.IsDir() {
			if err = root.MkdirAll(filepath.FromSlash(name), 0755); err != nil {
				return errors.New("插件 ZIP 目录冲突")
			}
			continue
		}
		if entry.UncompressedSize64 > 64<<20-total {
			return errors.New("插件解压后超过 64 MiB")
		}
		if err = root.MkdirAll(filepath.Dir(filepath.FromSlash(name)), 0755); err != nil {
			return errors.New("插件 ZIP 目录无效")
		}
		source, err := entry.Open()
		if err != nil {
			return errors.New("无法读取插件 ZIP")
		}
		target, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644|mode.Perm()&0111)
		if err != nil {
			source.Close()
			return errors.New("插件 ZIP 文件冲突")
		}
		n, copyErr := io.Copy(target, io.LimitReader(source, int64((64<<20)-total)+1))
		source.Close()
		closeErr := target.Close()
		if copyErr != nil || closeErr != nil || uint64(n) != entry.UncompressedSize64 {
			return errors.New("插件 ZIP 内容损坏或大小不符")
		}
		total += uint64(n)
		if name == "manifest.json" {
			manifestFound = true
		}
	}
	if !manifestFound {
		return errors.New("ZIP 根目录缺少 manifest.json")
	}
	return nil
}
