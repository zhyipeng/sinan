package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func encryptedFixture(t *testing.T) (*EncryptedDocuments, string, string) {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "encrypted-data")
	key := filepath.Join(root, "keys", "plugin-data.key")
	return OpenEncryptedDocuments(data, key), data, key
}
func TestEncryptedDocumentsRoundTripAndConflict(t *testing.T) {
	e, data, key := encryptedFixture(t)
	empty, err := e.Get("example.one")
	if err != nil || empty.Revision != 0 || string(empty.Value) != "null" {
		t.Fatal("empty encrypted document failed")
	}
	value := json.RawMessage(`{"fixture":"private-fixture-value"}`)
	doc, err := e.Put("example.one", 0, value)
	if err != nil || doc.Revision != 1 {
		t.Fatal("encrypted write failed", err)
	}
	file, err := os.ReadFile(filepath.Join(data, "example.one.json"))
	if err != nil || bytes.Contains(file, []byte("private-fixture-value")) {
		t.Fatal("plaintext leaked to ciphertext file")
	}
	persisted, err := OpenEncryptedDocuments(data, key).Get("example.one")
	if err != nil || !bytes.Equal(persisted.Value, value) || persisted.Revision != 1 {
		t.Fatal("restart did not decrypt document")
	}
	if _, err = e.Put("example.one", 0, json.RawMessage(`null`)); err == nil {
		t.Fatal("stale write accepted")
	}
	another, err := e.Get("example.two")
	if err != nil || another.Revision != 0 {
		t.Fatal("namespace isolation failed")
	}
	if _, err = e.Put("../escape", 0, value); err == nil {
		t.Fatal("invalid namespace accepted")
	}
	if _, err = e.Put("example.two", 0, json.RawMessage(`{"oversize":"`+string(bytes.Repeat([]byte("x"), MaxEncryptedDocumentBytes))+`"}`)); err == nil {
		t.Fatal("oversized plaintext accepted")
	}
	clearDoc, err := e.Put("example.one", 1, json.RawMessage(`null`))
	if err != nil || clearDoc.Revision != 2 {
		t.Fatal("encrypted clear failed")
	}
}
func TestEncryptedDocumentsRejectTamperingAndKeyLoss(t *testing.T) {
	for _, mutation := range []string{"ciphertext", "revision", "namespace", "key-loss", "key-corruption"} {
		t.Run(mutation, func(t *testing.T) {
			e, data, key := encryptedFixture(t)
			if _, err := e.Put("example.one", 0, json.RawMessage(`{"fixture":true}`)); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(data, "example.one.json")
			original, _ := os.ReadFile(path)
			var disk Document
			json.Unmarshal(original, &disk)
			switch mutation {
			case "ciphertext":
				var env encryptedEnvelope
				json.Unmarshal(disk.Value, &env)
				env.Ciphertext[0] ^= 1
				disk.Value, _ = json.Marshal(env)
			case "revision":
				disk.Revision++
			case "namespace":
				path = filepath.Join(data, "example.two.json")
			case "key-loss":
				if err := os.Remove(key); err != nil {
					t.Fatal(err)
				}
			case "key-corruption":
				if err := os.WriteFile(key, []byte("incomplete"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "ciphertext" || mutation == "revision" || mutation == "namespace" {
				encoded, _ := json.Marshal(disk)
				if err := os.WriteFile(path, encoded, 0600); err != nil {
					t.Fatal(err)
				}
			}
			id := "example.one"
			if mutation == "namespace" {
				id = "example.two"
			}
			if _, err := e.Get(id); err == nil {
				t.Fatal("tampered document was decrypted")
			}
			before, _ := os.ReadFile(path)
			if _, err := e.Put(id, disk.Revision, json.RawMessage(`null`)); err == nil {
				t.Fatal("corrupt storage was overwritten")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("corrupt original was modified")
			}
			if mutation == "key-loss" {
				if _, err := e.Put("example.new", 0, json.RawMessage(`null`)); err == nil {
					t.Fatal("lost installation key was silently regenerated")
				}
			}
		})
	}
}
func TestEncryptedDocumentsNoncesAndConcurrentRevisions(t *testing.T) {
	e, data, _ := encryptedFixture(t)
	value := json.RawMessage(`{"fixture":true}`)
	if _, err := e.Put("example.one", 0, value); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(data, "example.one.json"))
	if _, err := e.Put("example.one", 1, value); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(data, "example.one.json"))
	var firstDoc, secondDoc Document
	var firstEnvelope, secondEnvelope encryptedEnvelope
	json.Unmarshal(first, &firstDoc)
	json.Unmarshal(second, &secondDoc)
	json.Unmarshal(firstDoc.Value, &firstEnvelope)
	json.Unmarshal(secondDoc.Value, &secondEnvelope)
	if bytes.Equal(firstEnvelope.Nonce, secondEnvelope.Nonce) {
		t.Fatal("nonce reused between writes")
	}
	if bytes.Equal(first, second) {
		t.Fatal("encryption reused envelope")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for range 8 {
		wg.Go(func() {
			if _, err := e.Put("example.one", 2, value); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("concurrent stale writers succeeded")
	}
}
func TestEncryptedDocumentsRejectKeySymlink(t *testing.T) {
	e, _, key := encryptedFixture(t)
	if err := os.MkdirAll(filepath.Dir(key), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(target, bytes.Repeat([]byte("x"), 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, key); err != nil {
		t.Skip("current platform cannot create test symlink")
	}
	if _, err := e.Put("example.one", 0, json.RawMessage(`null`)); err == nil {
		t.Fatal("key symlink accepted")
	}
}

func TestHostVaultSharesKeyWithoutPluginAuthority(t *testing.T) {
	e, _, key := encryptedFixture(t)
	if _, err := e.PutHost("llm", 0, json.RawMessage(`{"fixture":"host-private"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Get("$llm"); err == nil {
		t.Fatal("ordinary read accepted host namespace")
	}
	if _, err := e.Put("$llm", 1, json.RawMessage(`null`)); err == nil {
		t.Fatal("ordinary write accepted host namespace")
	}
	if err := e.Clear("$llm", strings.Repeat("0", 64)); err == nil {
		t.Fatal("ordinary clear accepted host namespace")
	}
	rows, err := e.List()
	if err != nil || len(rows) != 0 {
		t.Fatal("host state appeared as plugin data")
	}
	if err = os.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Put("example.new", 0, json.RawMessage(`null`)); err == nil {
		t.Fatal("new plugin silently replaced key while host ciphertext exists")
	}
}
