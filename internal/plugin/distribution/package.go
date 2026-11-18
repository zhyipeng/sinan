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
	"sync"
	"time"
	"unicode/utf8"

	"sinan/internal/plugin/ghmirror"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*/[A-Za-z0-9_-][A-Za-z0-9_.-]*$`)

type Artifact struct {
	Repository string
	ID         int64
	Size       int64
	SHA256     string
}
type Fetcher struct {
	mu     sync.Mutex
	Client *http.Client
	cfg    ghmirror.Config
}

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

// downloadRedirect allows redirects to approved GitHub asset hosts and to the
// same host as the first request (so mirror services that canonicalise the path
// keep working). Other cross-host redirects are rejected.
func downloadRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("下载重定向过多")
	}
	if allowedURL(req.URL) {
		return nil
	}
	if len(via) > 0 && req.URL.Scheme == "https" && req.URL.User == nil && req.URL.Port() == "" && strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
		return nil
	}
	return errors.New("下载重定向被拒绝")
}

func New() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: downloadRedirect}, cfg: ghmirror.Config{Mode: "official"}}
}

// SetConfig applies the mirror preference to asset downloads.
func (f *Fetcher) SetConfig(cfg ghmirror.Config) {
	f.mu.Lock()
	f.cfg = cfg
	f.mu.Unlock()
}

func (f *Fetcher) config() ghmirror.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cfg
}

func (f *Fetcher) Stage(ctx context.Context, cacheDir string, a Artifact) (directory string, cleanup func(), err error) {
	return f.StageWithProgress(ctx, cacheDir, a, nil)
}

// StageWithProgress reports bytes written for the current download attempt.
// A mirror retry resets the count; total comes from the reviewed catalog asset.
func (f *Fetcher) StageWithProgress(ctx context.Context, cacheDir string, a Artifact, progress func(int64, int64)) (directory string, cleanup func(), err error) {
	if a.ID <= 0 || a.Size <= 0 || a.Size > 64<<20 || len(a.SHA256) != 64 || strings.Trim(a.SHA256, "0123456789abcdef") != "" || !repositoryPattern.MatchString(a.Repository) {
		return "", nil, errors.New("发行资产参数无效")
	}
	rawURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", a.Repository, a.ID)
	candidates := f.config().Candidates(rawURL)
	if len(candidates) == 0 {
		candidates = []string{rawURL}
	}
	var lastErr error
	for _, address := range candidates {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		if progress != nil {
			progress(0, a.Size)
		}
		staged, stagedCleanup, stageErr := f.stageFrom(ctx, cacheDir, a, address, progress)
		if stageErr == nil {
			return staged, stagedCleanup, nil
		}
		lastErr = stageErr
	}
	if lastErr == nil {
		lastErr = errors.New("插件包下载失败")
	}
	return "", nil, lastErr
}

// stageFrom downloads and validates one candidate URL. On failure it removes its
// own temporary directory so the next candidate starts clean.
func (f *Fetcher) stageFrom(ctx context.Context, cacheDir string, a Artifact, address string, progress func(int64, int64)) (string, func(), error) {
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return "", nil, errors.New("无法创建下载缓存")
	}
	work, err := os.MkdirTemp(cacheDir, "package-")
	if err != nil {
		return "", nil, errors.New("无法创建下载目录")
	}
	removeWork := func() { _ = os.RemoveAll(work) }
	success := false
	defer func() {
		if !success {
			removeWork()
		}
	}()
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return "", nil, errors.New("下载请求无效")
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "Sinan-Plugin-Store")
	response, err := f.Client.Do(req)
	if err != nil {
		return "", nil, downloadError(ctx, err, "插件包下载失败")
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
	n, copyErr := io.Copy(io.MultiWriter(file, hash, &downloadProgress{total: a.Size, report: progress}), io.LimitReader(response.Body, a.Size+1))
	closeErr := file.Close()
	var writeErr *os.PathError
	if closeErr != nil || errors.As(copyErr, &writeErr) {
		return "", nil, errors.New("无法写入下载缓存")
	}
	if copyErr != nil {
		return "", nil, downloadError(ctx, copyErr, "插件包下载中断")
	}
	if n != a.Size || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return "", nil, errors.New("插件包大小或 SHA-256 校验失败")
	}
	directory := filepath.Join(work, "plugin")
	if err = os.Mkdir(directory, 0700); err != nil {
		return "", nil, err
	}
	if err = extract(archive, directory); err != nil {
		return "", nil, err
	}
	_ = os.Remove(archive)
	success = true
	return directory, removeWork, nil
}

func downloadError(ctx context.Context, err error, message string) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return errors.New("插件包下载超时，请重试")
	}
	return errors.New(message)
}

type downloadProgress struct {
	written int64
	total   int64
	report  func(int64, int64)
}

func (p *downloadProgress) Write(data []byte) (int, error) {
	p.written += int64(len(data))
	if p.report != nil {
		p.report(min(p.written, p.total), p.total)
	}
	return len(data), nil
}

// StageLocal snapshots a local ZIP before extracting it with the same limits as store assets.
// Callers must clean up after both preview and apply; no source files are modified.
func StageLocal(source string) (directory string, cleanup func(), err error) {
	if !filepath.IsAbs(source) || !strings.EqualFold(filepath.Ext(source), ".zip") {
		return "", nil, errors.New("请选择 ZIP 插件包的绝对路径")
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		return "", nil, errors.New("ZIP 插件包不可读取或超过 64 MiB")
	}
	file, err := os.Open(source)
	if err != nil {
		return "", nil, errors.New("无法读取 ZIP 插件包")
	}
	defer file.Close()
	work, err := os.MkdirTemp("", "sinan-local-package-")
	if err != nil {
		return "", nil, errors.New("无法创建插件暂存目录")
	}
	cleanup = func() { _ = os.RemoveAll(work) }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()
	archive := filepath.Join(work, "package.zip")
	target, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", cleanup, errors.New("无法暂存 ZIP 插件包")
	}
	n, copyErr := io.Copy(target, io.LimitReader(file, (64<<20)+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil || n != info.Size() || n > 64<<20 {
		return "", cleanup, errors.New("ZIP 插件包读取失败、已变化或超过 64 MiB")
	}
	directory = filepath.Join(work, "plugin")
	if err = os.Mkdir(directory, 0700); err != nil {
		return "", cleanup, errors.New("无法创建插件暂存目录")
	}
	if err = extract(archive, directory); err != nil {
		return "", cleanup, err
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
