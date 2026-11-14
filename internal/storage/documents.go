package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

const MaxDocumentBytes = 32 << 10

var documentID = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$`)

type Document struct {
	Revision uint64          `json:"revision"`
	Value    json.RawMessage `json:"value"`
}

// Documents stores one bounded JSON document per plugin, outside installed packages.
type Documents struct {
	mu             sync.Mutex
	root           string
	hostNamespaces bool
}

func OpenDocuments(root string) *Documents { return &Documents{root: root} }
func (d *Documents) path(id string) (string, error) {
	if len(id) > 128 || (!documentID.MatchString(id) && !(d.hostNamespaces && hostDocumentID.MatchString(id))) {
		return "", errors.New("数据命名空间无效")
	}
	if err := os.MkdirAll(d.root, 0700); err != nil {
		return "", errors.New("无法准备插件数据目录")
	}
	info, err := os.Lstat(d.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("插件数据目录无效")
	}
	return filepath.Join(d.root, id+".json"), nil
}
func readDocument(path string) (Document, error) {
	empty := Document{Value: json.RawMessage("null")}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*MaxDocumentBytes+1024 {
		return empty, errors.New("插件数据文件无效，已保留原文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return empty, errors.New("无法读取插件数据")
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 2*MaxDocumentBytes+1025))
	decoder.DisallowUnknownFields()
	var doc Document
	if decoder.Decode(&doc) != nil || decoder.Decode(new(any)) != io.EOF || !json.Valid(doc.Value) || len(doc.Value) > MaxDocumentBytes {
		return empty, errors.New("插件数据损坏，已保留原文件")
	}
	return doc, nil
}
func (d *Documents) Get(id string) (Document, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	path, err := d.path(id)
	if err != nil {
		return Document{}, err
	}
	return readDocument(path)
}
func (d *Documents) Put(id string, revision uint64, value json.RawMessage) (Document, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(value) > MaxDocumentBytes || !json.Valid(value) {
		return Document{}, errors.New("插件数据须为不超过 32 KiB 的 JSON")
	}
	path, err := d.path(id)
	if err != nil {
		return Document{}, err
	}
	previous, err := readDocument(path)
	if err != nil {
		return Document{}, err
	}
	if previous.Revision != revision || revision == math.MaxUint64 {
		return Document{}, errors.New("数据已变化，请重新载入后保存")
	}
	next := Document{Revision: revision + 1, Value: append(json.RawMessage(nil), value...)}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(next)
	data := buffer.Bytes()
	if err != nil {
		return Document{}, errors.New("插件数据无效")
	}
	f, err := os.CreateTemp(d.root, ".document-")
	if err != nil {
		return Document{}, errors.New("无法创建数据文件")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return Document{}, errors.New("插件数据保存失败")
	}
	return next, nil
}
