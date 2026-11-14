package kernel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"sinan/internal/llm"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

type llmTask struct {
	owner       *uiview.Session
	cancel      context.CancelFunc
	expires     time.Time
	done        bool
	text, error string
	generation  uint64
}
type llmTasks struct {
	mu    sync.Mutex
	items map[string]*llmTask
}

func (t *llmTasks) cancelAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, job := range t.items {
		job.cancel()
		delete(t.items, id)
	}
}
func (m *Manager) LLMSettings() (llm.Snapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.closed {
		return llm.Snapshot{}, errors.New("运行时已关闭")
	}
	return m.llm.Snapshot()
}
func (m *Manager) SaveLLMProfile(edit llm.Edit) (llm.Snapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.closed {
		return llm.Snapshot{}, errors.New("运行时已关闭")
	}
	v, e := m.llm.Save(edit)
	if e == nil {
		m.llmTasks.cancelAll()
	}
	return v, e
}
func (m *Manager) DeleteLLMProfile(id string, revision uint64) (llm.Snapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.closed {
		return llm.Snapshot{}, errors.New("运行时已关闭")
	}
	v, e := m.llm.Delete(id, revision)
	if e == nil {
		m.llmTasks.cancelAll()
	}
	return v, e
}
func (m *Manager) SaveLLMPreferences(p llm.Preferences) (llm.Snapshot, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if m.closed {
		return llm.Snapshot{}, errors.New("运行时已关闭")
	}
	for id := range p.Overrides {
		if !manifest.ValidID(id) {
			return llm.Snapshot{}, errors.New("插件标识无效")
		}
	}
	v, e := m.llm.Preferences(p)
	if e == nil {
		m.llmTasks.cancelAll()
	}
	return v, e
}
func (m *Manager) callLLM(session *uiview.Session, pluginID string, r protocol.UIRequest) (json.RawMessage, error) {
	if r.Method == "llm.config" {
		value, e := m.llm.Choices(pluginID)
		if e != nil {
			return nil, e
		}
		return marshalUIResult(value)
	}
	m.llmTasks.mu.Lock()
	defer m.llmTasks.mu.Unlock()
	for id, job := range m.llmTasks.items {
		if time.Now().After(job.expires) || !job.owner.Active() {
			job.cancel()
			delete(m.llmTasks.items, id)
		}
	}
	if r.Method == "llm.poll" || r.Method == "llm.cancel" {
		job := m.llmTasks.items[r.Input]
		if job == nil || job.owner != session {
			return nil, errors.New("LLM 任务已失效")
		}
		if r.Method == "llm.cancel" {
			job.cancel()
			delete(m.llmTasks.items, r.Input)
			return marshalUIResult(nil)
		}
		if job.generation != m.llm.Generation() {
			job.cancel()
			delete(m.llmTasks.items, r.Input)
			return nil, errors.New("LLM 配置已更新，请重新请求")
		}
		return marshalUIResult(map[string]any{"done": job.done, "text": job.text, "error": job.error})
	}
	if r.Method != "llm.start" {
		return nil, errors.New("LLM 方法无效")
	}
	var request llm.Request
	decoder := json.NewDecoder(bytes.NewReader(r.Value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || len(request.SystemPrompt) > 8<<10 || len(request.Input) > 20<<10 || request.Input == "" {
		return nil, errors.New("LLM 请求无效或超出限制")
	}
	if _, _, e := m.llm.Resolve(pluginID, request.ProfileID); e != nil {
		return nil, e
	}
	count := 0
	for _, job := range m.llmTasks.items {
		if job.owner == session {
			count++
		}
	}
	if count >= 2 || len(m.llmTasks.items) >= 16 {
		return nil, errors.New("LLM 任务过多，请稍后重试")
	}
	bytes := make([]byte, 16)
	if _, e := rand.Read(bytes); e != nil {
		return nil, errors.New("无法创建 LLM 任务")
	}
	id := hex.EncodeToString(bytes)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	unsubscribe := session.OnRevoke(cancel)
	job := &llmTask{owner: session, cancel: cancel, expires: time.Now().Add(time.Minute), generation: m.llm.Generation()}
	m.llmTasks.items[id] = job
	go func() {
		defer cancel()
		defer unsubscribe()
		text, err := m.llm.Complete(ctx, pluginID, request)
		m.llmTasks.mu.Lock()
		defer m.llmTasks.mu.Unlock()
		if m.llmTasks.items[id] != job {
			return
		}
		job.done = true
		if err != nil {
			job.error = err.Error()
		} else if ctx.Err() != nil {
			job.error = "LLM 请求已取消"
		} else {
			job.text = text
		}
	}()
	return marshalUIResult(map[string]string{"id": id})
}
