package kernel

import (
	"errors"
	"sinan/internal/plugin/manifest"
	"sinan/internal/storage"
	"strings"
)

type PluginDataInfo struct {
	storage.DocumentInfo
	Encrypted bool   `json:"encrypted,omitempty"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// PluginData exposes metadata to the trusted management UI, never document values.
func (m *Manager) PluginData() ([]PluginDataInfo, error) {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, errors.New("运行时已关闭")
	}
	documents, err := m.documents.List()
	if err != nil {
		return nil, err
	}
	encrypted, err := m.encryptedDocuments.List()
	if err != nil {
		return nil, err
	}
	result := make([]PluginDataInfo, 0, len(documents)+len(encrypted))
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, doc := range documents {
		info := PluginDataInfo{DocumentInfo: doc, Name: doc.ID}
		if p := m.plugins[doc.ID]; p != nil {
			info.Name = p.manifest.Name
			info.Installed = true
		}
		result = append(result, info)
	}
	for _, doc := range encrypted {
		doc.Token = "encrypted:" + doc.Token
		info := PluginDataInfo{DocumentInfo: doc, Name: doc.ID, Encrypted: true}
		if p := m.plugins[doc.ID]; p != nil {
			info.Name = p.manifest.Name
			info.Installed = true
		}
		result = append(result, info)
	}
	return result, nil
}

// ClearPluginData serializes cleanup against writes and lifecycle mutations.
// Old sessions are revoked before releasing the mutation lock, so delayed
// autosaves cannot recreate the cleared document (including revision-zero ABA).
func (m *Manager) ClearPluginData(id, token string) error {
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	if !manifest.ValidID(id) {
		return errors.New("插件 ID 无效")
	}
	m.mu.Lock()
	closed, p := m.closed, m.plugins[id]
	m.mu.Unlock()
	if closed {
		return errors.New("运行时已关闭")
	}
	var err error
	if strings.HasPrefix(token, "encrypted:") {
		err = m.encryptedDocuments.Clear(id, strings.TrimPrefix(token, "encrypted:"))
	} else {
		err = m.documents.Clear(id, token)
	}
	if err != nil {
		return err
	}
	if p != nil {
		p.mu.Lock()
		p.revokeViewsLocked()
		p.mu.Unlock()
	}
	return nil
}
