package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentCleanupReviewAndMetadata(t *testing.T) {
	d := OpenDocuments(t.TempDir())
	d.Put("example.a", 0, json.RawMessage(`{"draft":"private fixture"}`))
	list, err := d.List()
	if err != nil || len(list) != 1 || list[0].Corrupt || list[0].Token == "" || list[0].Bytes <= 0 {
		t.Fatal("bad metadata", err)
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "private fixture") {
		t.Fatal("document value exposed")
	}
	token := list[0].Token
	if _, err = d.Put("example.a", 1, json.RawMessage(`{"draft":"updated"}`)); err != nil {
		t.Fatal(err)
	}
	if err = d.Clear("example.a", token); err == nil {
		t.Fatal("stale cleanup review accepted")
	}
	current, err := d.Get("example.a")
	if err != nil || current.Revision != 2 {
		t.Fatal("failed cleanup changed document", err)
	}
	list, _ = d.List()
	if err = d.Clear("example.a", list[0].Token); err != nil {
		t.Fatal(err)
	}
	list, _ = d.List()
	if len(list) != 0 {
		t.Fatal("cleared document still listed")
	}
	current, err = d.Get("example.a")
	if err != nil || current.Revision != 0 || string(current.Value) != "null" {
		t.Fatal("bad reset", err)
	}
	if err = d.Clear("../escape", strings.Repeat("a", 64)); err == nil {
		t.Fatal("path accepted")
	}
}
func TestCleanupCorruptAndOversizedFilesWithoutFollowingLinks(t *testing.T) {
	root := t.TempDir()
	d := OpenDocuments(root)
	if err := os.WriteFile(filepath.Join(root, "example.corrupt.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	// Larger than the normal read budget; administration still reads bounded data.
	if err := os.WriteFile(filepath.Join(root, "example.large.json"), []byte(strings.Repeat("x", 1<<20)), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("do not remove"), 0600)
	if err := os.Symlink(target, filepath.Join(root, "example.link.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "example.directory.json"), 0700); err != nil {
		t.Fatal(err)
	}
	list, err := d.List()
	if err != nil || len(list) != 4 {
		t.Fatal("missing entries", err)
	}
	for _, row := range list {
		if !row.Corrupt {
			t.Fatal("corrupt document reported healthy")
		}
		if row.ID == "example.link" || row.ID == "example.directory" {
			if row.Token != "" {
				t.Fatal("unsafe entry clearable")
			}
			if err := d.Clear(row.ID, strings.Repeat("a", 64)); err == nil {
				t.Fatal("unsafe entry removed")
			}
		} else if err := d.Clear(row.ID, row.Token); err != nil {
			t.Fatal(err)
		}
	}
	bytes, err := os.ReadFile(target)
	if err != nil || string(bytes) != "do not remove" {
		t.Fatal("linked file changed", err)
	}
}

func TestCleanupReviewRejectsReplacementWithSameContentAndTimestamp(t *testing.T) {
	root := t.TempDir()
	d := OpenDocuments(root)
	if _, err := d.Put("example.same", 0, json.RawMessage(`{"text":"same"}`)); err != nil {
		t.Fatal(err)
	}
	reviewed, err := d.List()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "example.same.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err = d.Clear("example.same", reviewed[0].Token); err == nil {
		t.Fatal("old review accepted a different file")
	}
	current, err := d.List()
	if err != nil || len(current) != 1 {
		t.Fatal(err)
	}
	if err = d.Clear("example.same", current[0].Token); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupKeepsUnselectedDocument(t *testing.T) {
	d := OpenDocuments(t.TempDir())
	if _, err := d.Put("example.a", 0, json.RawMessage(`"a"`)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Put("example.b", 0, json.RawMessage(`"b"`)); err != nil {
		t.Fatal(err)
	}
	reviewed, err := d.List()
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Clear("example.a", reviewed[0].Token); err != nil {
		t.Fatal(err)
	}
	other, err := d.Get("example.b")
	if err != nil || other.Revision != 1 || string(other.Value) != `"b"` {
		t.Fatal("other document changed", err)
	}
	remaining, err := d.List()
	if err != nil || len(remaining) != 1 || remaining[0].ID != "example.b" {
		t.Fatal("cleanup removed unselected entries", err)
	}
}
