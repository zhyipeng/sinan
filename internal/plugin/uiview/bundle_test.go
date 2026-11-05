package uiview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBundleOwnershipCanonicalURLsAndImmutableBytes(t *testing.T) {
	source := fstest.MapFS{"index.html": {Data: []byte("<h1>fixture</h1>")}, "assets/main.js": {Data: []byte("fixture()")}}
	b, err := LoadFS(context.Background(), source, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"file:///etc/passwd", "https://example.test/index.html", "sinan-plugin://example.other/index.html", "sinan-plugin://example.ui@elsewhere/index.html", "sinan-plugin://example.ui:80/index.html", "sinan-plugin://example.ui/../index.html", "sinan-plugin://example.ui/%69ndex.html", "sinan-plugin://example.ui/assets/%2e%2e/index.html", "sinan-plugin://example.ui/index.html?x=1", "sinan-plugin://example.ui/index.html#x", "sinan-plugin://example.ui/private.json"} {
		if _, ok := b.Resource(raw); ok {
			t.Fatal("invalid resource URL accepted", raw)
		}
	}
	r, ok := b.Resource(b.EntryURL())
	if !ok || r.MIME != "text/html" {
		t.Fatal("entry missing")
	}
	r.Data[0] = 'X'
	source["index.html"].Data[0] = 'Y'
	r, _ = b.Resource(b.EntryURL())
	if string(r.Data) != "<h1>fixture</h1>" {
		t.Fatal("snapshot changed through caller alias")
	}
}

func TestBundleRejectsUnsupportedFilesAndBudgets(t *testing.T) {
	cases := []fstest.MapFS{
		{"index.html": {Data: []byte("ok")}, "secret.env": {Data: []byte("fixture")}},
		{"index.html": {Data: []byte(strings.Repeat("x", MaxFileBytes+1))}},
		{"index.html": {Data: []byte("ok")}, "bad path.js": {Data: []byte("fixture")}},
	}
	tooMany := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	for i := 0; i < MaxFiles; i++ {
		tooMany[fmt.Sprintf("%d.js", i)] = &fstest.MapFile{Data: []byte("fixture")}
	}
	cases = append(cases, tooMany)
	tooLarge := fstest.MapFS{"index.html": {Data: []byte("ok")}}
	for i := 0; i < 5; i++ {
		tooLarge[fmt.Sprintf("%d.js", i)] = &fstest.MapFile{Data: []byte(strings.Repeat("x", MaxFileBytes))}
	}
	cases = append(cases, tooLarge)
	for i, source := range cases {
		if _, err := LoadFS(context.Background(), source, "example.ui", "index.html"); err == nil {
			t.Fatal("invalid bundle accepted", i)
		}
	}
	if _, err := LoadFS(context.Background(), fstest.MapFS{}, "example.ui", "index.html"); err == nil {
		t.Fatal("missing entry accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadFS(ctx, fstest.MapFS{}, "example.ui", "index.html"); err == nil {
		t.Fatal("cancelled load accepted")
	}
}

func TestDirectorySnapshotRejectsSymlinksAndSurvivesSourceChanges(t *testing.T) {
	dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectory(context.Background(), alias, "example.ui", "index.html"); err == nil {
		t.Fatal("symlink root accepted")
	}
	entry := filepath.Join(dir, "index.html")
	if err := os.WriteFile(entry, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := LoadDirectory(context.Background(), dir, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	r, ok := b.Resource(b.EntryURL())
	if !ok || string(r.Data) != "fixture" {
		t.Fatal("snapshot still reads source files")
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.html"), entry); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectory(context.Background(), dir, "example.ui", "index.html"); err == nil {
		t.Fatal("symlink accepted")
	}
}
