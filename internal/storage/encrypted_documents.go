package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
)

// The sealed JSON envelope must also fit the ordinary document's 32 KiB limit.
const MaxEncryptedDocumentBytes = 20 << 10

type encryptedEnvelope struct {
	Version    int    `json:"version"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// EncryptedDocuments encrypts each plugin's private document with an installation
// key. This protects ciphertext alone, not a user who can also read the key.
// The caller must hold the installation's existing exclusive root lease.
type EncryptedDocuments struct {
	mu        sync.Mutex
	documents *Documents
	keyPath   string
}

func OpenEncryptedDocuments(root, keyPath string) *EncryptedDocuments {
	documents := OpenDocuments(root)
	documents.hostNamespaces = true
	return &EncryptedDocuments{documents: documents, keyPath: keyPath}
}
func encryptedError() error {
	return errors.New("加密存储不可用、密钥丢失或数据损坏，已保留原文件")
}
func privateDirectory(path string) error {
	if os.MkdirAll(path, 0700) != nil {
		return encryptedError()
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return encryptedError()
	}
	if restrictPrivate(path, true) != nil {
		return encryptedError()
	}
	return nil
}
func (e *EncryptedDocuments) aead(create bool) (cipher.AEAD, error) {
	if privateDirectory(filepath.Dir(e.keyPath)) != nil {
		return nil, encryptedError()
	}
	info, err := os.Lstat(e.keyPath)
	if os.IsNotExist(err) && create {
		// A missing key must never be silently replaced while ciphertext still exists.
		entries, readErr := os.ReadDir(e.documents.root)
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, encryptedError()
		}
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".json" {
				return nil, encryptedError()
			}
		}
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, encryptedError()
		}
		f, openErr := os.OpenFile(e.keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			return nil, encryptedError()
		}
		err = restrictPrivate(e.keyPath, false)
		if err == nil {
			_, err = f.Write(key)
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return nil, encryptedError()
		}
		info, err = os.Lstat(e.keyPath)
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() != 32 {
		return nil, encryptedError()
	}
	if restrictPrivate(e.keyPath, false) != nil {
		return nil, encryptedError()
	}
	f, err := os.Open(e.keyPath)
	if err != nil {
		return nil, encryptedError()
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, encryptedError()
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		return nil, encryptedError()
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, encryptedError()
	}
	return cipher.NewGCM(block)
}
func encryptedAAD(id string, revision uint64) []byte {
	return []byte("sinan.encrypted-document.v1\x00" + id + "\x00" + strconv.FormatUint(revision, 10))
}
func (e *EncryptedDocuments) get(id string) (Document, error) {
	raw, err := e.documents.Get(id)
	if err != nil {
		return Document{}, encryptedError()
	}
	if raw.Revision == 0 && bytes.Equal(raw.Value, []byte("null")) {
		return raw, nil
	}
	aead, err := e.aead(false)
	if err != nil {
		return Document{}, err
	}
	var envelope encryptedEnvelope
	if json.Unmarshal(raw.Value, &envelope) != nil || envelope.Version != 1 || len(envelope.Nonce) != aead.NonceSize() {
		return Document{}, encryptedError()
	}
	value, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, encryptedAAD(id, raw.Revision))
	if err != nil || len(value) > MaxEncryptedDocumentBytes || !json.Valid(value) {
		return Document{}, encryptedError()
	}
	return Document{Revision: raw.Revision, Value: value}, nil
}

var hostDocumentID = regexp.MustCompile(`^\$[a-z][a-z0-9-]{0,63}$`)

func (e *EncryptedDocuments) Get(id string) (Document, error) {
	if !documentID.MatchString(id) {
		return Document{}, encryptedError()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.get(id)
}
func (e *EncryptedDocuments) Put(id string, revision uint64, value json.RawMessage) (Document, error) {
	if !documentID.MatchString(id) {
		return Document{}, encryptedError()
	}
	return e.put(id, revision, value)
}
func (e *EncryptedDocuments) put(id string, revision uint64, value json.RawMessage) (Document, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !json.Valid(value) || len(value) > MaxEncryptedDocumentBytes {
		return Document{}, errors.New("加密数据须为不超过 20 KiB 的 JSON")
	}
	previous, err := e.get(id)
	if err != nil {
		return Document{}, err
	}
	if previous.Revision != revision || revision == math.MaxUint64 {
		return Document{}, errors.New("加密数据已变化，请重新载入后保存")
	}
	if privateDirectory(e.documents.root) != nil {
		return Document{}, encryptedError()
	}
	aead, err := e.aead(true)
	if err != nil {
		return Document{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return Document{}, encryptedError()
	}
	ciphertext := aead.Seal(nil, nonce, value, encryptedAAD(id, revision+1))
	encoded, err := json.Marshal(encryptedEnvelope{Version: 1, Nonce: nonce, Ciphertext: ciphertext})
	if err != nil {
		return Document{}, encryptedError()
	}
	next, err := e.documents.Put(id, revision, encoded)
	if err != nil {
		return Document{}, err
	}
	return Document{Revision: next.Revision, Value: append(json.RawMessage(nil), value...)}, nil
}

// List exposes metadata only; authenticating values never exposes them to callers.
func (e *EncryptedDocuments) List() ([]DocumentInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rows, err := e.documents.List()
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if _, err := e.get(rows[i].ID); err != nil {
			rows[i].Corrupt = true
		}
	}
	return rows, nil
}
func (e *EncryptedDocuments) Clear(id, token string) error {
	if !documentID.MatchString(id) {
		return encryptedError()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.documents.Clear(id, token)
}

// Host namespaces cannot be represented by a plugin manifest ID or listed as plugin data.
func (e *EncryptedDocuments) GetHost(name string) (Document, error) {
	id := "$" + name
	if !hostDocumentID.MatchString(id) {
		return Document{}, encryptedError()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.get(id)
}
func (e *EncryptedDocuments) PutHost(name string, revision uint64, value json.RawMessage) (Document, error) {
	id := "$" + name
	if !hostDocumentID.MatchString(id) {
		return Document{}, encryptedError()
	}
	return e.put(id, revision, value)
}
