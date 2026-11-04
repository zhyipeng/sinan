package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type SettingsSnapshot struct {
	Revision uint64              `json:"revision"`
	Values   map[string][]string `json:"values"`
}
type settingsDocument struct {
	Version int                         `json:"version"`
	Plugins map[string]SettingsSnapshot `json:"plugins"`
}
type Settings struct {
	mu       sync.Mutex
	path     string
	document settingsDocument
	loadErr  error
}

func OpenSettings(path string) *Settings {
	s := &Settings{path: path, document: settingsDocument{Version: 1, Plugins: map[string]SettingsSnapshot{}}}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s
	}
	if err != nil {
		s.loadErr = errors.New("无法读取设置文件")
		return s
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		s.loadErr = errors.New("设置文件读取失败或超过大小限制")
		return s
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var doc settingsDocument
	if decoder.Decode(&doc) != nil || decoder.Decode(new(any)) != io.EOF || doc.Plugins == nil || len(doc.Plugins) > 256 {
		s.loadErr = errors.New("设置文件无效，已保留原文件")
		return s
	}
	// Version 0 used the same namespace layout without a version marker.
	if doc.Version == 0 {
		doc.Version = 1
	}
	if doc.Version != 1 {
		s.loadErr = errors.New("设置文件版本不受支持，已保留原文件")
		return s
	}
	s.document = doc
	return s
}
func (s *Settings) Error() error { return s.loadErr }
func copySettings(value SettingsSnapshot) SettingsSnapshot {
	copy := SettingsSnapshot{Revision: value.Revision, Values: map[string][]string{}}
	for key, values := range value.Values {
		copy.Values[key] = append([]string{}, values...)
	}
	return copy
}
func (s *Settings) Get(pluginID string) SettingsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copySettings(s.document.Plugins[pluginID])
}
func (s *Settings) Update(pluginID string, revision uint64, values map[string][]string) (SettingsSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return SettingsSnapshot{}, s.loadErr
	}
	previous, exists := s.document.Plugins[pluginID]
	if previous.Revision != revision {
		return SettingsSnapshot{}, errors.New("设置已变化，请重新打开设置后重试")
	}
	if !exists && len(s.document.Plugins) >= 256 {
		return SettingsSnapshot{}, errors.New("设置命名空间数量超过限制")
	}
	next := copySettings(SettingsSnapshot{Revision: revision + 1, Values: values})
	s.document.Plugins[pluginID] = next
	err := s.saveLocked()
	if err != nil {
		if exists {
			s.document.Plugins[pluginID] = previous
		} else {
			delete(s.document.Plugins, pluginID)
		}
		return SettingsSnapshot{}, errors.New("设置保存失败，请检查本地存储后重试")
	}
	return copySettings(next), nil
}
func (s *Settings) saveLocked() error {
	data, err := json.Marshal(s.document)
	if err != nil || len(data) > 1<<20 {
		return errors.New("settings too large")
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}
