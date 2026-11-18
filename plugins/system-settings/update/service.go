// Package update owns the settings plugin's desktop release-checking policy.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"sinan/internal/buildinfo"
	"sinan/internal/hostupdate"
)

type State struct {
	CurrentVersion     string `json:"currentVersion"`
	LatestVersion      string `json:"latestVersion"`
	Phase              string `json:"phase"`
	Message            string `json:"message"`
	Available          bool   `json:"available"`
	Supported          bool   `json:"supported"`
	InstallationResult string `json:"installationResult"`
	DownloadedBytes    int64  `json:"downloadedBytes"`
	TotalBytes         int64  `json:"totalBytes"`
}
type candidate struct {
	version, url, checksum string
	size                   int64
}
type Service struct {
	mu        sync.Mutex
	state     State
	candidate *candidate
	target    string
	client    *http.Client
	endpoint  string
	changed   func()
}

func New(executable, current string, changed func()) *Service {
	s := &Service{state: State{CurrentVersion: current, Phase: "idle"}, changed: changed, endpoint: "https://api.github.com/repos/" + buildinfo.Repository + "/releases/latest", client: &http.Client{Timeout: 10 * time.Minute, CheckRedirect: redirect}}
	target, err := hostupdate.Installation(executable, current)
	if err != nil {
		s.state.Phase = "unsupported"
		s.state.Message = "开发构建或非完整发行包不支持自动更新"
	} else {
		s.target = target
		s.state.Supported = true
	}
	return s
}
func redirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 || req.URL.Scheme != "https" || req.URL.User != nil {
		return errors.New("更新下载重定向被拒绝")
	}
	switch req.URL.Host {
	case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return nil
	}
	return errors.New("更新下载来源不受支持")
}
func (s *Service) State() State {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	state.InstallationResult = hostupdate.LastResult()
	return state
}
func (s *Service) emit() {
	if s.changed != nil {
		s.changed()
	}
}

// start is called with mu held.
func (s *Service) start(phase string) error {
	if !s.state.Supported {
		return errors.New("此运行模式不支持自动更新")
	}
	switch s.state.Phase {
	case "checking", "downloading", "preparing", "installing":
		return errors.New("更新操作正在进行")
	}
	s.state.Phase = phase
	s.state.Message = ""
	s.state.DownloadedBytes = 0
	s.state.TotalBytes = 0
	return nil
}
func (s *Service) fail(message string) {
	s.mu.Lock()
	s.state.Phase = "error"
	s.state.Message = message
	s.mu.Unlock()
	s.emit()
}
func (s *Service) request(ctx context.Context, raw string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("更新请求无效")
	}
	req.Header.Set("User-Agent", "Sinan-Desktop-Updater")
	if req.URL.Host == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
	} else {
		req.Header.Set("Accept", "application/octet-stream")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 GitHub 更新服务")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("更新服务暂不可用（HTTP %d）", response.StatusCode)
	}
	return response, nil
}
func (s *Service) read(ctx context.Context, raw string, limit int64) ([]byte, error) {
	response, err := s.request(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("更新元数据超限或读取失败")
	}
	return data, nil
}
func checksumFor(data []byte, name string) (string, error) {
	found := ""
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 16 {
		return "", errors.New("更新摘要清单超限")
	}
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 || len(parts[0]) != 64 {
			return "", errors.New("更新摘要清单格式无效")
		}
		hash, err := hex.DecodeString(parts[0])
		if err != nil || len(hash) != 32 {
			return "", errors.New("更新摘要格式无效")
		}
		if parts[1] == name {
			if found != "" {
				return "", errors.New("更新摘要重复")
			}
			found = strings.ToLower(parts[0])
		}
	}
	if found == "" {
		return "", errors.New("更新摘要缺少当前平台")
	}
	return found, nil
}
func (s *Service) Check(ctx context.Context) (State, error) {
	s.mu.Lock()
	err := s.start("checking")
	s.mu.Unlock()
	if err != nil {
		return s.State(), err
	}
	s.emit()
	err = s.check(ctx)
	if err != nil {
		s.fail(err.Error())
	}
	return s.State(), err
}
func (s *Service) check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := s.read(ctx, s.endpoint, 1<<20)
	if err != nil {
		return err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if json.Unmarshal(data, &release) != nil || !strings.HasPrefix(release.Tag, "v") {
		return errors.New("更新版本信息无效")
	}
	version := strings.TrimPrefix(release.Tag, "v")
	if !buildinfo.ValidVersion(version) || release.Draft || release.Prerelease || strings.Contains(version, "-") {
		return errors.New("更新服务没有提供有效稳定版本")
	}
	s.mu.Lock()
	current := s.state.CurrentVersion
	s.mu.Unlock()
	if buildinfo.Compare(version, current) <= 0 {
		s.mu.Lock()
		s.candidate = nil
		s.state.LatestVersion = ""
		s.state.Available = false
		s.state.Phase = "current"
		s.state.Message = "已是最新版本"
		s.mu.Unlock()
		s.emit()
		return nil
	}
	name := hostupdate.AssetName(version)
	base := "https://github.com/" + buildinfo.Repository + "/releases/download/" + url.PathEscape(release.Tag) + "/"
	var assetSize int64
	archiveFound, checksumFound := false, false
	for _, asset := range release.Assets {
		switch asset.Name {
		case name:
			if archiveFound || asset.URL != base+name || asset.Size <= 0 || asset.Size > hostupdate.MaxArchive {
				return errors.New("更新资产来源或大小无效")
			}
			archiveFound = true
			assetSize = asset.Size
		case "SHA256SUMS.txt":
			if checksumFound || asset.URL != base+asset.Name || asset.Size <= 0 || asset.Size > 8192 {
				return errors.New("更新摘要资产无效")
			}
			checksumFound = true
		}
	}
	if !archiveFound || !checksumFound {
		return errors.New("新版缺少当前平台发行包或 SHA-256 摘要")
	}
	checksums, err := s.read(ctx, base+"SHA256SUMS.txt", 8192)
	if err != nil {
		return err
	}
	hash, err := checksumFor(checksums, name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.candidate = &candidate{version: version, url: base + name, checksum: hash, size: assetSize}
	s.state.LatestVersion = version
	s.state.Available = true
	s.state.Phase = "available"
	s.mu.Unlock()
	s.emit()
	return nil
}

// StartInstall reserves the single update task before returning. ctx belongs
// to the application, and handoff takes ownership of the plan on success.
func (s *Service) StartInstall(ctx context.Context, handoff func(hostupdate.Plan) error) error {
	if ctx.Err() != nil {
		return errors.New("更新已取消")
	}
	s.mu.Lock()
	if s.candidate == nil && s.state.Supported {
		s.mu.Unlock()
		return errors.New("请先检查更新")
	}
	if err := s.start("downloading"); err != nil {
		s.mu.Unlock()
		return err
	}
	candidate := s.candidate
	current := s.state.CurrentVersion
	s.state.TotalBytes = candidate.size
	s.mu.Unlock()
	go s.runInstall(ctx, *candidate, current, handoff)
	return nil
}

func (s *Service) runInstall(ctx context.Context, candidate candidate, current string, handoff func(hostupdate.Plan) error) {
	s.emit()
	plan, err := s.download(ctx, candidate, current)
	if ctx.Err() != nil {
		plan.Cleanup()
		s.fail("更新已取消")
		return
	}
	if err != nil {
		s.fail(err.Error())
		return
	}
	s.setPhase("installing")
	if err := handoff(plan); err != nil {
		plan.Cleanup()
		s.fail("更新助手未就绪，应用未退出，请重试")
	}
}

func (s *Service) setPhase(phase string) {
	s.mu.Lock()
	s.state.Phase = phase
	s.mu.Unlock()
	s.emit()
}

type progressWriter struct {
	io.Writer
	advance func(int64)
}

func (w progressWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.advance(int64(n))
	return n, err
}

func (s *Service) download(ctx context.Context, candidate candidate, current string) (hostupdate.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	root, err := hostupdate.CacheRoot()
	if err != nil {
		return hostupdate.Plan{}, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return hostupdate.Plan{}, errors.New("无法创建更新缓存")
	}
	file, err := os.CreateTemp(root, "download-")
	if err != nil {
		return hostupdate.Plan{}, errors.New("无法创建下载文件")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	response, err := s.request(ctx, candidate.url)
	if err != nil {
		return hostupdate.Plan{}, err
	}
	defer response.Body.Close()
	hasher := sha256.New()
	writer := progressWriter{Writer: io.MultiWriter(file, hasher), advance: func(n int64) {
		s.mu.Lock()
		s.state.DownloadedBytes = min(s.state.DownloadedBytes+n, candidate.size)
		s.mu.Unlock()
	}}
	n, err := io.Copy(writer, io.LimitReader(response.Body, candidate.size+1))
	if err != nil || n != candidate.size || hex.EncodeToString(hasher.Sum(nil)) != candidate.checksum {
		return hostupdate.Plan{}, errors.New("下载不完整或 SHA-256 校验失败，未安装")
	}
	if file.Close() != nil {
		return hostupdate.Plan{}, errors.New("无法保存下载文件")
	}
	if ctx.Err() != nil {
		return hostupdate.Plan{}, errors.New("更新已取消或超时，请重试")
	}
	s.setPhase("preparing")
	plan, err := hostupdate.StageArchive(file.Name(), s.target, candidate.version, current)
	if err != nil {
		return hostupdate.Plan{}, errors.New("更新准备失败：" + safeStageError(err))
	}
	return plan, nil
}
func safeStageError(err error) string {
	// Filesystem errors may contain local paths; never forward raw OS errors.
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "无法读写应用目录，请检查权限和磁盘空间"
	}
	return err.Error()
}
