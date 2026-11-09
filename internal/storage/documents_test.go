package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentsIsolationRevisionAndCorruption(t *testing.T) {
	root := t.TempDir()
	d := OpenDocuments(root)
	a, err := d.Get("example.a")
	if err != nil || a.Revision != 0 || string(a.Value) != "null" {
		t.Fatal("bad empty document", err)
	}
	a, err = d.Put("example.a", 0, json.RawMessage(`{"text":"draft"}`))
	if err != nil || a.Revision != 1 {
		t.Fatal(err)
	}
	if _, err = d.Put("example.a", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("stale revision accepted")
	}
	other, _ := d.Get("example.b")
	if other.Revision != 0 {
		t.Fatal("cross-plugin document leak")
	}
	restarted := OpenDocuments(root)
	a, err = restarted.Get("example.a")
	if err != nil || a.Revision != 1 || string(a.Value) != `{"text":"draft"}` {
		t.Fatal("restart lost data", err)
	}
	for _, id := range []string{"../escape", "$launcher", "example.a/other"} {
		if _, err = d.Get(id); err == nil {
			t.Fatal("path accepted")
		}
	}
	if _, err = d.Put("example.a", 1, json.RawMessage(`"`+strings.Repeat("x", MaxDocumentBytes)+`"`)); err == nil {
		t.Fatal("oversize accepted")
	}
	path := filepath.Join(root, "example.a.json")
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Put("example.a", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("corruption overwritten")
	}
	bytes, _ := os.ReadFile(path)
	if string(bytes) != "broken" {
		t.Fatal("original lost")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte(`{"revision":1,"value":null}`), 0600)
	if err = os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Get("example.a"); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestDocumentHTMLCharactersDoNotExpandBeyondQuota(t *testing.T) {
	d := OpenDocuments(t.TempDir())
	value := json.RawMessage(`"` + strings.Repeat("<>&", 10000) + `"`)
	if _, err := d.Put("example.html", 0, value); err != nil {
		t.Fatal(err)
	}
	doc, err := d.Get("example.html")
	if err != nil || string(doc.Value) != string(value) {
		t.Fatal("accepted document unreadable after serialization", err)
	}
}
