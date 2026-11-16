// Package catalog owns the plugin store's fixed GitHub directory contract.
package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sinan/internal/buildinfo"
	"sinan/internal/plugin/ghmirror"
	"sinan/internal/plugin/icon"
	"sinan/internal/plugin/manifest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Repository = "zhyipeng/sinan-plugin-registry"

// Development builds expose the capabilities of the current source baseline.
func HostVersion() string {
	if buildinfo.Version == "dev" {
		return "0.2.1"
	}
	if !buildinfo.ValidVersion(buildinfo.Version) {
		return "0.0.0"
	}
	return buildinfo.Version
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*/[A-Za-z0-9_-][A-Za-z0-9_.-]*$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Publisher struct {
	Kind string `json:"kind"`
}
type Source struct {
	Repository *string `json:"repository"`
	Path       string  `json:"path"`
}
type Compatibility struct {
	API string `json:"apiVersion"`
	UI  string `json:"uiBridgeVersion"`
}
type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	ID     int64  `json:"assetId"`
	URL    string `json:"downloadUrl"`
}
type Release struct {
	SchemaVersion  int           `json:"schemaVersion"`
	PluginID       string        `json:"pluginId"`
	Name           string        `json:"name"`
	Version        string        `json:"version"`
	Repository     string        `json:"repository"`
	Tag            string        `json:"tag"`
	Commit         string        `json:"commit"`
	ReleaseID      int64         `json:"releaseId"`
	MinHostVersion string        `json:"minHostVersion"`
	Compatibility  Compatibility `json:"compatibility"`
	Assets         []Asset       `json:"assets"`
}
type Entry struct {
	Icon          string    `json:"icon,omitempty"`
	SchemaVersion int       `json:"schemaVersion"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Summary       string    `json:"summary"`
	Publisher     Publisher `json:"publisher"`
	Source        Source    `json:"source"`
	Status        string    `json:"status"`
	Releases      []Release `json:"releases"`
}
type Snapshot struct {
	Entries   []Entry `json:"entries"`
	Revision  string  `json:"revision"`
	UpdatedAt int64   `json:"updatedAt"`
	Stale     bool    `json:"stale"`
	Notice    string  `json:"notice"`
	ETag      string  `json:"etag,omitempty"`
}

func validVersion(version string) bool {
	if !versionPattern.MatchString(version) || len(version) > 32 {
		return false
	}
	for _, part := range strings.Split(version, ".") {
		if len(part) > 1 && part[0] == '0' {
			return false
		}
		if _, err := strconv.ParseUint(part, 10, 53); err != nil {
			return false
		}
	}
	return true
}

func CompareVersions(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		if len(aa) != 3 || len(bb) != 3 {
			return 0
		}
		av, _ := strconv.ParseUint(aa[i], 10, 64)
		bv, _ := strconv.ParseUint(bb[i], 10, 64)
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// Tags are repository-wide; namespacing lets plugins version independently.
// Legacy tags remain valid so existing catalog records keep working.
func validReleaseTag(pluginID, version, tag string) bool {
	return tag == "v"+version || tag == pluginID+"-v"+version
}

func (r Release) Compatible() bool {
	return r.Compatibility.API == "1" && (r.Compatibility.UI == "1" || r.Compatibility.UI == "2") && validVersion(r.MinHostVersion) && buildinfo.Compare(HostVersion(), r.MinHostVersion) >= 0
}
func (r Release) Asset(osName, arch string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.OS == osName && a.Arch == arch {
			return a, true
		}
	}
	return Asset{}, false
}
func Validate(e Entry) error {
	if e.Icon != "" && icon.Normalize(e.Icon) == "" {
		return errors.New("目录图标无效")
	}
	if e.SchemaVersion != 1 || !manifest.ValidID(e.ID) || strings.HasPrefix(e.ID, "system.") || strings.TrimSpace(e.Name) == "" || len([]rune(e.Name)) > 64 || len(e.Summary) > 2048 {
		return errors.New("插件资料无效")
	}
	if e.Publisher.Kind != "official" && e.Publisher.Kind != "community" {
		return errors.New("发布者类型无效")
	}
	if e.Status != "pending-release" && e.Status != "published" && e.Status != "withdrawn" {
		return errors.New("目录状态无效")
	}
	if len(e.Releases) > 64 {
		return errors.New("版本记录过多")
	}
	if e.Source.Repository != nil && !repositoryPattern.MatchString(strings.TrimPrefix(*e.Source.Repository, "https://github.com/")) {
		return errors.New("源码仓库无效")
	}
	if e.Source.Repository != nil && !strings.HasPrefix(*e.Source.Repository, "https://github.com/") {
		return errors.New("源码地址无效")
	}
	if e.Source.Path == "" || strings.HasPrefix(e.Source.Path, "/") || strings.ContainsAny(e.Source.Path, "\\:\x00") {
		return errors.New("源码路径无效")
	}
	for _, part := range strings.Split(e.Source.Path, "/") {
		if part == ".." {
			return errors.New("源码路径无效")
		}
	}
	seen := map[string]bool{}
	for _, r := range e.Releases {
		if r.SchemaVersion != 1 || r.PluginID != e.ID || strings.TrimSpace(r.Name) == "" || len([]rune(r.Name)) > 64 || !validVersion(r.Version) || !validVersion(r.MinHostVersion) || !repositoryPattern.MatchString(r.Repository) || !validReleaseTag(r.PluginID, r.Version, r.Tag) || r.ReleaseID <= 0 || seen[r.Version] || len(r.Assets) == 0 || len(r.Assets) > 8 {
			return errors.New("发布版本记录无效")
		}
		if e.Source.Repository == nil || *e.Source.Repository != "https://github.com/"+r.Repository {
			return errors.New("发行仓库与源码仓库不一致")
		}
		seen[r.Version] = true
		if len(r.Commit) != 40 || strings.Trim(r.Commit, "0123456789abcdef") != "" {
			return errors.New("源码提交无效")
		}
		targets := map[string]bool{}
		for _, a := range r.Assets {
			expected := fmt.Sprintf("%s-%s-%s-%s.zip", e.ID, r.Version, a.OS, a.Arch)
			target := a.OS + "/" + a.Arch
			if (a.OS != "darwin" && a.OS != "windows" && a.OS != "linux") || (a.Arch != "arm64" && a.Arch != "amd64") || targets[target] || a.ID <= 0 || a.Size <= 0 || a.Size > 64<<20 || !shaPattern.MatchString(a.SHA256) || a.File != expected || a.URL != "https://github.com/"+r.Repository+"/releases/download/"+r.Tag+"/"+a.File {
				return errors.New("发行资产记录无效")
			}
			targets[target] = true
		}
	}
	if e.Status == "pending-release" && len(e.Releases) > 0 || e.Status == "published" && len(e.Releases) == 0 {
		return errors.New("目录状态与版本不一致")
	}
	return nil
}
func decode(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("JSON 包含额外内容")
	}
	return nil
}

type Client struct {
	mu        sync.Mutex
	http      *http.Client
	base      string
	cacheFile string
	cached    Snapshot
	cfg       ghmirror.Config
}

func New(cacheDir string) *Client {
	return &Client{
		http:      &http.Client{Timeout: 15 * time.Second, CheckRedirect: sameHostRedirect},
		base:      "https://api.github.com",
		cacheFile: filepath.Join(cacheDir, "catalog.json"),
		cfg:       ghmirror.Config{Mode: "official"},
	}
}

// SetConfig applies the mirror preference. The default (zero) preference keeps
// the official endpoint only; the host supplies "auto" when the setting is unset.
func (c *Client) SetConfig(cfg ghmirror.Config) {
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
}

// sameHostRedirect permits HTTPS redirects only when they stay on the host of
// the first request in the chain (e.g. github.chenc.dev canonicalising to
// /https:/...). Cross-host redirects remain rejected.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("目录重定向次数过多")
	}
	if req.URL.Scheme != "https" || req.URL.User != nil {
		return errors.New("目录不接受此重定向")
	}
	if len(via) > 0 && !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
		return errors.New("目录不接受跨主机重定向")
	}
	return nil
}

var (
	errCatalogRateLimited = errors.New("catalog rate limited")
	errCatalogUnavailable = errors.New("catalog unavailable")
)

func (c *Client) read(ctx context.Context, path, etag string) ([]byte, string, bool, error) {
	rawURL := c.base + path
	candidates := c.cfg.Candidates(rawURL)
	if len(candidates) == 0 {
		candidates = []string{rawURL}
	}
	rateLimited, unavailable, connectFailed := false, false, false
	for _, target := range candidates {
		data, newETag, unchanged, err := c.readOnce(ctx, target, etag)
		if err == nil {
			return data, newETag, unchanged, nil
		}
		switch {
		case errors.Is(err, errCatalogRateLimited):
			rateLimited = true
		case errors.Is(err, errCatalogUnavailable):
			unavailable = true
		default:
			connectFailed = true
		}
	}
	switch {
	case rateLimited:
		return nil, "", false, errors.New("GitHub 请求受限，请稍后刷新")
	case unavailable:
		return nil, "", false, errors.New("插件目录暂不可用")
	case connectFailed:
		return nil, "", false, errors.New("无法连接 GitHub 插件目录")
	default:
		return nil, "", false, errors.New("插件目录暂不可用")
	}
}

func (c *Client) readOnce(ctx context.Context, target, etag string) ([]byte, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, "", false, errors.New("目录请求无效")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Sinan-Plugin-Store")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == 304 {
		return nil, etag, true, nil
	}
	if response.StatusCode == 403 || response.StatusCode == 429 {
		return nil, "", false, errCatalogRateLimited
	}
	if response.StatusCode != 200 {
		return nil, "", false, errCatalogUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil || len(data) > 2<<20 {
		return nil, "", false, errors.New("插件目录响应超过限制")
	}
	return data, response.Header.Get("ETag"), false, nil
}
func (c *Client) Load(ctx context.Context, force bool) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached.Revision == "" {
		if data, err := os.ReadFile(c.cacheFile); err == nil && len(data) <= 2<<20 {
			var saved Snapshot
			if json.Unmarshal(data, &saved) == nil && saved.Revision != "" && len(saved.Entries) <= 128 {
				valid := true
				for _, entry := range saved.Entries {
					valid = valid && Validate(entry) == nil
				}
				if valid {
					c.cached = saved
				}
			}
		}
	}
	if !force && c.cached.Revision != "" && time.Now().UnixMilli()-c.cached.UpdatedAt < 300000 {
		return c.cached, nil
	}
	snapshot, err := c.fetch(ctx)
	if err != nil {
		if c.cached.Revision != "" {
			fallback := c.cached
			fallback.Stale = true
			fallback.Notice = err.Error() + "，正在显示缓存"
			return fallback, nil
		}
		return Snapshot{}, err
	}
	c.cached = snapshot
	if data, err := json.Marshal(snapshot); err == nil {
		if os.MkdirAll(filepath.Dir(c.cacheFile), 0700) == nil {
			if file, err := os.CreateTemp(filepath.Dir(c.cacheFile), ".catalog-"); err == nil {
				tmp := file.Name()
				if _, err = file.Write(data); err == nil {
					err = file.Close()
					if err == nil {
						_ = os.Rename(tmp, c.cacheFile)
					}
				} else {
					_ = file.Close()
				}
				_ = os.Remove(tmp)
			}
		}
	}
	return snapshot, nil
}
func (c *Client) fetch(ctx context.Context) (Snapshot, error) {
	data, etag, unchanged, err := c.read(ctx, "/repos/"+Repository+"/git/trees/main?recursive=1", c.cached.ETag)
	if err != nil {
		return Snapshot{}, err
	}
	if unchanged {
		if c.cached.Revision == "" {
			return Snapshot{}, errors.New("目录缓存不可用")
		}
		snapshot := c.cached
		snapshot.UpdatedAt = time.Now().UnixMilli()
		snapshot.Stale = false
		snapshot.Notice = ""
		return snapshot, nil
	}
	var tree struct {
		SHA       string `json:"sha"`
		Truncated bool   `json:"truncated"`
		Items     []struct {
			Path string `json:"path"`
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if json.Unmarshal(data, &tree) != nil || tree.Truncated || len(tree.SHA) != 40 || strings.Trim(tree.SHA, "0123456789abcdef") != "" {
		return Snapshot{}, errors.New("GitHub 目录树无效或不完整")
	}
	snapshot := Snapshot{Entries: []Entry{}, Revision: tree.SHA, UpdatedAt: time.Now().UnixMilli(), ETag: etag}
	totalBytes := 0
	for _, item := range tree.Items {
		parts := strings.Split(item.Path, "/")
		if len(parts) != 3 || parts[0] != "plugins" || parts[2] != "plugin.json" || item.Type != "blob" {
			continue
		}
		if !manifest.ValidID(parts[1]) || len(item.SHA) != 40 || strings.Trim(item.SHA, "0123456789abcdef") != "" || len(snapshot.Entries) >= 128 {
			return Snapshot{}, errors.New("插件目录记录超过限制或路径无效")
		}
		data, _, _, err := c.read(ctx, "/repos/"+Repository+"/git/blobs/"+item.SHA, "")
		if err != nil {
			return Snapshot{}, err
		}
		var blob struct {
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		}
		if json.Unmarshal(data, &blob) != nil || blob.Encoding != "base64" {
			return Snapshot{}, errors.New("插件资料编码无效")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(blob.Content, "\n", ""))
		totalBytes += len(raw)
		if err != nil || len(raw) > 128<<10 || totalBytes > 2<<20 {
			return Snapshot{}, errors.New("插件资料超过限制")
		}
		var entry Entry
		if decode(raw, &entry) != nil || entry.ID != parts[1] || Validate(entry) != nil {
			return Snapshot{}, errors.New("插件资料不符合目录协议")
		}
		sort.Slice(entry.Releases, func(i, j int) bool { return CompareVersions(entry.Releases[i].Version, entry.Releases[j].Version) > 0 })
		snapshot.Entries = append(snapshot.Entries, entry)
	}
	sort.Slice(snapshot.Entries, func(i, j int) bool { return snapshot.Entries[i].ID < snapshot.Entries[j].ID })
	return snapshot, nil
}
