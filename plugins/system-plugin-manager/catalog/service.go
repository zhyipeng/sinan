package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"runtime"
	"sinan/internal/kernel"
	"sinan/internal/plugin/distribution"
	"sinan/internal/plugin/ghmirror"
	"sync"
	"time"
)

type Backend interface {
	Snapshot() kernel.Snapshot
	PreviewPackage(string) (kernel.PackagePreview, error)
	ApplyPackage(string, bool, string, string) (kernel.PluginInfo, error)
}
type Loader interface {
	Load(context.Context, bool) (Snapshot, error)
}
type Stager interface {
	Stage(context.Context, string, distribution.Artifact) (string, func(), error)
}
type progressStager interface {
	StageWithProgress(context.Context, string, distribution.Artifact, func(int64, int64)) (string, func(), error)
}
type Item struct {
	Entry
	InstalledVersion string `json:"installedVersion"`
	Development      bool   `json:"development"`
	Changed          bool   `json:"changed"`
	Protected        bool   `json:"protected"`
}
type View struct {
	Entries     []Item `json:"entries"`
	Revision    string `json:"revision"`
	UpdatedAt   int64  `json:"updatedAt"`
	Stale       bool   `json:"stale"`
	Notice      string `json:"notice"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	HostVersion string `json:"hostVersion"`
}
type Preview struct {
	kernel.PackagePreview
	Ticket        string `json:"ticket"`
	ExpiresAt     int64  `json:"expiresAt"`
	Repository    string `json:"repository"`
	ArchiveSHA256 string `json:"archiveSha256"`
}
type pending struct {
	preview   Preview
	entry     Entry
	release   Release
	asset     Asset
	directory string
	cleanup   func()
	expires   time.Time
}
type Service struct {
	tasks     installTasks
	Loader    Loader
	Fetcher   Stager
	Backend   Backend
	CacheDir  string
	mu        sync.Mutex
	pending   map[string]pending
	preparing int
	closed    bool
}

func NewService(backend Backend, cacheDir string) *Service {
	return &Service{Loader: New(cacheDir), Fetcher: distribution.New(), Backend: backend, CacheDir: cacheDir, pending: map[string]pending{}}
}

// SetMirrorConfig applies the mirror preference to catalog reads and package
// downloads. It is cheap and safe to call on every store request, so setting
// changes take effect without recreating the service.
func (s *Service) SetMirrorConfig(cfg ghmirror.Config) {
	if client, ok := s.Loader.(*Client); ok {
		client.SetConfig(cfg)
	}
	if fetcher, ok := s.Fetcher.(*distribution.Fetcher); ok {
		fetcher.SetConfig(cfg)
	}
}
func (s *Service) Load(ctx context.Context, force bool) (View, error) {
	snapshot, err := s.Loader.Load(ctx, force)
	if err != nil {
		return View{}, err
	}
	installed := map[string]kernel.PluginInfo{}
	for _, p := range s.Backend.Snapshot().Plugins {
		installed[p.ID] = p
	}
	result := View{Entries: []Item{}, Revision: snapshot.Revision, UpdatedAt: snapshot.UpdatedAt, Stale: snapshot.Stale, Notice: snapshot.Notice, OS: runtime.GOOS, Arch: runtime.GOARCH, HostVersion: HostVersion()}
	for _, entry := range snapshot.Entries {
		p := installed[entry.ID]
		if entry.Icon == "" {
			entry.Icon = p.Icon
		}
		result.Entries = append(result.Entries, Item{Entry: entry, InstalledVersion: p.Version, Development: p.Development, Changed: p.Changed, Protected: p.Protected})
	}
	return result, nil
}
func find(snapshot Snapshot, id, version string) (Entry, Release, Asset, error) {
	for _, entry := range snapshot.Entries {
		if entry.ID != id {
			continue
		}
		if entry.Status != "published" {
			return Entry{}, Release{}, Asset{}, errors.New("该插件尚未发布或已下架")
		}
		for _, release := range entry.Releases {
			if release.Version == version {
				if !release.Compatible() {
					return Entry{}, Release{}, Asset{}, errors.New("该版本与当前宿主不兼容")
				}
				if asset, ok := release.Asset(runtime.GOOS, runtime.GOARCH); ok {
					return entry, release, asset, nil
				}
			}
		}
	}
	return Entry{}, Release{}, Asset{}, errors.New("当前平台没有可安装版本")
}
func (s *Service) Prepare(ctx context.Context, id, version, revision string) (Preview, error) {
	return s.prepare(ctx, id, version, revision, nil)
}
func (s *Service) prepare(ctx context.Context, id, version, revision string, progress func(int64, int64)) (Preview, error) {
	s.mu.Lock()
	s.expireLocked()
	if s.closed || len(s.pending)+s.preparing >= 4 {
		s.mu.Unlock()
		return Preview{}, errors.New("安装检查繁忙，请取消旧预览后重试")
	}
	s.preparing++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.preparing--; s.mu.Unlock() }()
	snapshot, err := s.Loader.Load(ctx, true)
	if err != nil || snapshot.Stale {
		return Preview{}, errors.New("无法核验最新插件目录，请稍后重试")
	}
	if snapshot.Revision != revision {
		return Preview{}, errors.New("目录已更新，请刷新商店后重新选择")
	}
	entry, release, asset, err := find(snapshot, id, version)
	if err != nil {
		return Preview{}, err
	}
	for _, p := range s.Backend.Snapshot().Plugins {
		if p.ID == id && (p.Development || p.Changed || p.Protected) {
			return Preview{}, errors.New("本地插件为开发链接、存在修改或受保护，请先在插件管理中处理")
		}
	}
	artifact := distribution.Artifact{Repository: release.Repository, ID: asset.ID, Size: asset.Size, SHA256: asset.SHA256}
	var directory string
	var cleanup func()
	if fetcher, ok := s.Fetcher.(progressStager); ok && progress != nil {
		directory, cleanup, err = fetcher.StageWithProgress(ctx, s.CacheDir, artifact, progress)
	} else {
		directory, cleanup, err = s.Fetcher.Stage(ctx, s.CacheDir, artifact)
	}
	if err != nil {
		return Preview{}, err
	}
	accepted := false
	defer func() {
		if !accepted {
			cleanup()
		}
	}()
	preview, err := s.Backend.PreviewPackage(directory)
	if err != nil {
		return Preview{}, err
	}
	if preview.ID != id || preview.Version != version || preview.Name != release.Name {
		return Preview{}, errors.New("安装包身份与目录记录不一致")
	}
	token := make([]byte, 24)
	if _, err = rand.Read(token); err != nil {
		return Preview{}, errors.New("无法建立安装预览")
	}
	expires := time.Now().Add(10 * time.Minute)
	result := Preview{PackagePreview: preview, Ticket: hex.EncodeToString(token), ExpiresAt: expires.UnixMilli(), Repository: release.Repository, ArchiveSHA256: asset.SHA256}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Preview{}, errors.New("商店已关闭")
	}
	s.pending[result.Ticket] = pending{preview: result, entry: entry, release: release, asset: asset, directory: directory, cleanup: cleanup, expires: expires}
	accepted = true
	return result, nil
}
func (s *Service) Apply(ctx context.Context, ticket string, trust bool) (kernel.PluginInfo, error) {
	s.mu.Lock()
	s.expireLocked()
	p, ok := s.pending[ticket]
	if ok {
		delete(s.pending, ticket)
		s.preparing++
	}
	closed := s.closed
	s.mu.Unlock()
	if ok {
		defer func() { s.mu.Lock(); s.preparing--; s.mu.Unlock(); p.cleanup() }()
	}
	if !ok || closed {
		return kernel.PluginInfo{}, errors.New("安装预览已过期，请重新检查")
	}
	if p.preview.Kind == "process" && !trust {
		return kernel.PluginInfo{}, errors.New("请明确确认信任此原生插件")
	}
	snapshot, err := s.Loader.Load(ctx, true)
	if err != nil || snapshot.Stale {
		return kernel.PluginInfo{}, errors.New("无法核验最新目录，安装未应用")
	}
	_, release, asset, err := find(snapshot, p.preview.ID, p.preview.Version)
	if err != nil || release.Repository != p.release.Repository || release.ReleaseID != p.release.ReleaseID || asset != p.asset {
		return kernel.PluginInfo{}, errors.New("该发行资产已被撤回或替换，请重新检查")
	}
	return s.Backend.ApplyPackage(p.directory, trust, p.preview.Fingerprint, p.preview.InstalledFingerprint)
}
func (s *Service) Cancel(ticket string) {
	s.mu.Lock()
	p, ok := s.pending[ticket]
	delete(s.pending, ticket)
	s.mu.Unlock()
	if ok {
		p.cleanup()
	}
}
func (s *Service) expireLocked() {
	for ticket, p := range s.pending {
		if time.Now().After(p.expires) {
			delete(s.pending, ticket)
			p.cleanup()
		}
	}
}
func (s *Service) Close() {
	s.closeInstallTasks()
	s.mu.Lock()
	s.closed = true
	tickets := s.pending
	s.pending = map[string]pending{}
	s.mu.Unlock()
	for _, p := range tickets {
		p.cleanup()
	}
}
