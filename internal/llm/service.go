// Package llm supplies shared transport and configuration, without plugin prompts.
package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"sinan/internal/storage"
)

type Profile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseURL"`
	Model   string `json:"model"`
	HasKey  bool   `json:"hasKey"`
}
type credential struct {
	Profile
	APIKey string `json:"apiKey"`
}
type state struct {
	Profiles       []credential      `json:"profiles"`
	DefaultProfile string            `json:"defaultProfile"`
	Overrides      map[string]string `json:"overrides"`
}
type Snapshot struct {
	Revision       uint64            `json:"revision"`
	Profiles       []Profile         `json:"profiles"`
	DefaultProfile string            `json:"defaultProfile"`
	Overrides      map[string]string `json:"overrides"`
}
type Edit struct {
	Revision uint64 `json:"revision"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseURL"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey"`
	ClearKey bool   `json:"clearKey"`
}
type Preferences struct {
	Revision       uint64            `json:"revision"`
	DefaultProfile string            `json:"defaultProfile"`
	Overrides      map[string]string `json:"overrides"`
}
type Request struct {
	ProfileID    string `json:"profileId,omitempty"`
	SystemPrompt string `json:"systemPrompt"`
	Input        string `json:"input"`
}
type Choice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Choices struct {
	Profiles       []Choice `json:"profiles"`
	DefaultProfile string   `json:"defaultProfile"`
}
type Service struct {
	mu         sync.Mutex
	vault      *storage.EncryptedDocuments
	client     *http.Client
	generation uint64
}

func New(vault *storage.EncryptedDocuments) *Service {
	return &Service{vault: vault, client: &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *Service) read() (state, uint64, error) {
	doc, err := s.vault.GetHost("llm")
	if err != nil {
		return state{}, 0, err
	}
	v := state{Profiles: []credential{}, Overrides: map[string]string{}}
	if string(doc.Value) != "null" && json.Unmarshal(doc.Value, &v) != nil {
		return state{}, 0, errors.New("全局 LLM 配置损坏，已保留原文件")
	}
	if v.Overrides == nil {
		v.Overrides = map[string]string{}
	}
	return v, doc.Revision, nil
}
func public(v state, revision uint64) Snapshot {
	out := Snapshot{Revision: revision, Profiles: []Profile{}, DefaultProfile: v.DefaultProfile, Overrides: v.Overrides}
	for _, p := range v.Profiles {
		meta := p.Profile
		meta.HasKey = p.APIKey != ""
		out.Profiles = append(out.Profiles, meta)
	}
	return out
}
func (s *Service) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, r, e := s.read()
	return public(v, r), e
}
func (s *Service) write(v state, revision uint64) (Snapshot, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return Snapshot{}, errors.New("LLM 配置无效")
	}
	doc, err := s.vault.PutHost("llm", revision, data)
	if err != nil {
		return Snapshot{}, err
	}
	s.generation++
	return public(v, doc.Revision), nil
}
func Endpoint(p Profile) (string, error) {
	if strings.TrimSpace(p.Name) == "" || len([]rune(p.Name)) > 64 || strings.ContainsFunc(p.Name, unicode.IsControl) || strings.TrimSpace(p.Model) == "" || len(p.Model) > 128 || strings.ContainsFunc(p.Model, unicode.IsControl) {
		return "", errors.New("请输入有效的配置名称和模型")
	}
	if len(p.BaseURL) > 2048 {
		return "", errors.New("API 地址过长")
	}
	u, err := url.Parse(strings.TrimSpace(p.BaseURL))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("API 地址不能包含账号、查询参数或片段")
	}
	ip := net.ParseIP(u.Hostname())
	local := strings.EqualFold(u.Hostname(), "localhost") || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("远程 API 须使用 HTTPS，本机可使用 HTTP")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	if u.Path == "" {
		u.Path = "/v1"
	}
	if !strings.HasSuffix(u.Path, "/chat/completions") {
		u.Path += "/chat/completions"
	}
	return u.String(), nil
}
func (s *Service) Save(edit Edit) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, revision, err := s.read()
	if err != nil {
		return Snapshot{}, err
	}
	if revision != edit.Revision {
		return Snapshot{}, errors.New("LLM 配置已变化，请重新载入")
	}
	next := credential{Profile: Profile{ID: edit.ID, Name: strings.TrimSpace(edit.Name), BaseURL: strings.TrimSpace(edit.BaseURL), Model: strings.TrimSpace(edit.Model)}, APIKey: edit.APIKey}
	index := -1
	for i, p := range v.Profiles {
		if p.ID == edit.ID {
			index = i
		}
		if p.ID != edit.ID && strings.EqualFold(p.Name, next.Name) {
			return Snapshot{}, errors.New("配置名称已存在")
		}
	}
	if edit.ID != "" && index < 0 {
		return Snapshot{}, errors.New("配置已失效，请重新载入")
	}
	if index >= 0 {
		old := v.Profiles[index]
		if old.APIKey != "" && old.BaseURL != next.BaseURL && edit.APIKey == "" && !edit.ClearKey {
			return Snapshot{}, errors.New("修改 API 地址时须重新输入或清除 Key")
		}
		if edit.APIKey == "" {
			next.APIKey = old.APIKey
		}
	}
	if edit.ClearKey {
		next.APIKey = ""
	}
	if len(next.APIKey) > 2048 || strings.ContainsFunc(next.APIKey, unicode.IsControl) {
		return Snapshot{}, errors.New("API Key 格式无效")
	}
	if _, err = Endpoint(next.Profile); err != nil {
		return Snapshot{}, err
	}
	if index < 0 {
		if len(v.Profiles) >= 16 {
			return Snapshot{}, errors.New("最多创建 16 套 LLM 配置")
		}
		id := make([]byte, 16)
		if _, err = rand.Read(id); err != nil {
			return Snapshot{}, errors.New("无法生成配置标识")
		}
		next.ID = hex.EncodeToString(id)
		v.Profiles = append(v.Profiles, next)
	} else {
		v.Profiles[index] = next
	}
	if v.DefaultProfile == "" {
		v.DefaultProfile = next.ID
	}
	return s.write(v, revision)
}
func (s *Service) Delete(id string, expected uint64) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, r, e := s.read()
	if e != nil {
		return Snapshot{}, e
	}
	if r != expected {
		return Snapshot{}, errors.New("LLM 配置已变化，请重新载入")
	}
	found := false
	profiles := []credential{}
	for _, p := range v.Profiles {
		if p.ID == id {
			found = true
		} else {
			profiles = append(profiles, p)
		}
	}
	if !found {
		return Snapshot{}, errors.New("配置不存在")
	}
	v.Profiles = profiles
	if v.DefaultProfile == id {
		v.DefaultProfile = ""
		if len(profiles) > 0 {
			v.DefaultProfile = profiles[0].ID
		}
	}
	for plugin, value := range v.Overrides {
		if value == id {
			delete(v.Overrides, plugin)
		}
	}
	return s.write(v, r)
}
func (s *Service) Preferences(p Preferences) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, r, e := s.read()
	if e != nil {
		return Snapshot{}, e
	}
	if r != p.Revision {
		return Snapshot{}, errors.New("LLM 配置已变化，请重新载入")
	}
	exists := map[string]bool{"": true}
	for _, profile := range v.Profiles {
		exists[profile.ID] = true
	}
	if !exists[p.DefaultProfile] || len(v.Profiles) > 0 && p.DefaultProfile == "" {
		return Snapshot{}, errors.New("请选择默认配置")
	}
	if len(p.Overrides) > 128 {
		return Snapshot{}, errors.New("插件覆盖配置过多")
	}
	for _, id := range p.Overrides {
		if !exists[id] {
			return Snapshot{}, errors.New("插件覆盖配置已失效")
		}
	}
	v.DefaultProfile = p.DefaultProfile
	v.Overrides = p.Overrides
	return s.write(v, r)
}
func (s *Service) Choices(pluginID string) (Choices, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, _, e := s.read()
	out := Choices{Profiles: []Choice{}, DefaultProfile: v.DefaultProfile}
	if id := v.Overrides[pluginID]; id != "" {
		out.DefaultProfile = id
	}
	for _, p := range v.Profiles {
		out.Profiles = append(out.Profiles, Choice{p.ID, p.Name})
	}
	return out, e
}
func (s *Service) Resolve(pluginID, profileID string) (credential, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, _, e := s.read()
	if e != nil {
		return credential{}, 0, e
	}
	if profileID == "" {
		profileID = v.Overrides[pluginID]
		if profileID == "" {
			profileID = v.DefaultProfile
		}
	}
	for _, p := range v.Profiles {
		if p.ID == profileID {
			return p, s.generation, nil
		}
	}
	return credential{}, 0, errors.New("请在设置中创建并选择全局 LLM 配置")
}
func (s *Service) Generation() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.generation }
func (s *Service) Complete(ctx context.Context, pluginID string, r Request) (string, error) {
	if strings.TrimSpace(r.Input) == "" || len(r.Input) > 20<<10 || len(r.SystemPrompt) > 8<<10 {
		return "", errors.New("LLM 输入或提示词无效或超出限制")
	}
	p, _, err := s.Resolve(pluginID, r.ProfileID)
	if err != nil {
		return "", err
	}
	endpoint, err := Endpoint(p.Profile)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(map[string]any{"model": p.Model, "stream": false, "messages": []map[string]string{{"role": "system", "content": r.SystemPrompt}, {"role": "user", "content": r.Input}}})
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return "", errors.New("无法创建 LLM 请求")
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return "", errors.New("LLM 连接失败或超时")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return "", errors.New("LLM 认证失败，请检查 Key 和模型权限")
	}
	if res.StatusCode == 429 {
		return "", errors.New("LLM 额度不足或请求过于频繁")
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("LLM 服务返回 HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (256<<10)+1))
	if err != nil || len(body) > 256<<10 {
		return "", errors.New("LLM 响应读取失败或过大")
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &response) != nil || len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", errors.New("LLM 未返回兼容的文本结果")
	}
	output := response.Choices[0].Message.Content
	if len(output) > 64<<10 || p.APIKey != "" && strings.Contains(output, p.APIKey) {
		return "", errors.New("LLM 返回内容无效")
	}
	return output, nil
}
