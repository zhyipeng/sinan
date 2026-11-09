package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DocumentInfo never contains document values. Token binds an explicit cleanup
// review to file metadata and bounded content; it is not a page credential.
type DocumentInfo struct {
	ID      string `json:"id"`
	Bytes   int64  `json:"bytes"`
	Corrupt bool   `json:"corrupt"`
	Token   string `json:"token"`
}

func inspectDocument(path, id string) (DocumentInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return DocumentInfo{}, errors.New("插件数据已变化，请刷新后重试")
	}
	result := DocumentInfo{ID: id, Bytes: info.Size(), Corrupt: true}
	// Never follow links or enumerate a directory presented as a document.
	if !info.Mode().IsRegular() {
		result.Bytes = 0
		return result, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return result, nil
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return DocumentInfo{}, errors.New("插件数据正在变化，请刷新后重试")
	}
	identity, err := documentIdentity(f, opened)
	if err != nil {
		return result, nil
	}
	digest := sha256.New()
	header, _ := json.Marshal([]any{identity, info.Size(), info.ModTime().UnixNano(), info.Mode().String()})
	digest.Write(header)
	// Normal documents are hashed in full. Oversized/corrupt files use their
	// size, timestamp, first and last 64 KiB, keeping administrative reads bounded.
	if _, err = io.Copy(digest, io.LimitReader(f, 64<<10)); err != nil {
		return result, nil
	}
	if info.Size() > 64<<10 {
		if _, err = f.Seek(max(64<<10, info.Size()-(64<<10)), io.SeekStart); err != nil {
			return result, nil
		}
		if _, err = io.Copy(digest, io.LimitReader(f, 64<<10)); err != nil {
			return result, nil
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return DocumentInfo{}, errors.New("插件数据正在变化，请刷新后重试")
	}
	result.Token = hex.EncodeToString(digest.Sum(nil))
	if info.Size() <= 2*MaxDocumentBytes+1024 {
		_, err = readDocument(path)
		result.Corrupt = err != nil
	}
	return result, nil
}

func (d *Documents) List() ([]DocumentInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := []DocumentInfo{}
	info, err := os.Lstat(d.root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("插件数据目录无效")
	}
	dir, err := os.Open(d.root)
	if err != nil {
		return nil, errors.New("无法读取插件数据目录")
	}
	defer dir.Close()
	entries, err := dir.ReadDir(2049)
	if err != nil && err != io.EOF || len(entries) > 2048 {
		return nil, errors.New("插件数据目录读取失败或条目过多")
	}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if id == entry.Name() || len(id) > 128 || !documentID.MatchString(id) {
			continue
		}
		record, err := inspectDocument(filepath.Join(d.root, entry.Name()), id)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (d *Documents) Clear(id, token string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(token) != 64 {
		return errors.New("请先检查插件数据")
	}
	path, err := d.path(id)
	if err != nil {
		return err
	}
	current, err := inspectDocument(path, id)
	if err != nil {
		return err
	}
	if current.Token == "" {
		return errors.New("数据文件不可清理，请检查文件类型或权限")
	}
	if current.Token != token {
		return errors.New("插件数据已变化，请刷新后重新确认")
	}
	if err = os.Remove(path); err != nil {
		return errors.New("无法清空插件数据，原文件已保留")
	}
	return nil
}
