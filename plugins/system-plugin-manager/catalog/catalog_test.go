package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sinan/internal/plugin/ghmirror"
	"strings"
	"testing"
)

func TestGitHubSnapshotCacheAndOfflineFallback(t *testing.T) {
	s, loader, _, _ := serviceFixture(t)
	_ = s
	entry := loader.snapshot.Entries[0]
	raw, _ := json.Marshal(entry)
	treeSHA, blobSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	calls := 0
	offline := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if offline {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/repos/" + Repository + "/git/trees/main":
			if r.Header.Get("If-None-Match") == `"reviewed"` {
				w.WriteHeader(304)
				return
			}
			w.Header().Set("ETag", `"reviewed"`)
			json.NewEncoder(w).Encode(map[string]any{"sha": treeSHA, "tree": []map[string]string{{"path": "plugins/example.store/plugin.json", "sha": blobSHA, "type": "blob"}, {"path": "submissions/ignored/0.1.0.json", "sha": blobSHA, "type": "blob"}}})
		case "/repos/" + Repository + "/git/blobs/" + blobSHA:
			json.NewEncoder(w).Encode(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(raw)})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cache := t.TempDir()
	c := New(cache)
	c.base = server.URL
	c.http = server.Client()
	snapshot, err := c.Load(context.Background(), true)
	if err != nil || len(snapshot.Entries) != 1 || snapshot.Revision != treeSHA {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	if _, err = c.Load(context.Background(), false); err != nil || calls != 2 {
		t.Fatal("memory cache missed")
	}
	if _, err = c.Load(context.Background(), true); err != nil || calls != 3 {
		t.Fatal("conditional refresh failed")
	}
	offline = true
	disk := New(cache)
	disk.base = server.URL
	disk.http = server.Client()
	snapshot, err = disk.Load(context.Background(), true)
	if err != nil || !snapshot.Stale || len(snapshot.Entries) != 1 || snapshot.Notice == "" {
		t.Fatalf("offline cache: %+v %v", snapshot, err)
	}
	empty := New(filepath.Join(t.TempDir(), "empty"))
	empty.base = server.URL
	empty.http = server.Client()
	if _, err = empty.Load(context.Background(), true); err == nil {
		t.Fatal("missing cache hidden")
	}
}

func TestCatalogRejectsInvalidReleaseRecords(t *testing.T) {
	for _, kind := range []string{"overflow", "repository", "digest", "source", "status", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			_, loader, _, _ := serviceFixture(t)
			entry := loader.snapshot.Entries[0]
			if err := Validate(entry); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "overflow":
				entry.Releases[0].Version = "999999999999999999999999.0.0"
			case "repository":
				entry.Releases[0].Repository = "../plugin"
			case "digest":
				entry.Releases[0].Assets[0].SHA256 = "bad"
			case "source":
				entry.Source.Repository = nil
			case "status":
				entry.Status = "pending-release"
			case "duplicate":
				entry.Releases = append(entry.Releases, entry.Releases[0])
			}
			if Validate(entry) == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
}

func TestOptionalCatalogArtwork(t *testing.T) {
	_, loader, _, _ := serviceFixture(t)
	entry := loader.snapshot.Entries[0]
	if err := Validate(entry); err != nil {
		t.Fatal("old catalog entry no longer compatible", err)
	}
	entry.Icon = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><rect width="40" height="40" fill="#e9efff"/></svg>`))
	if err := Validate(entry); err != nil {
		t.Fatal("embedded catalog artwork rejected", err)
	}
	for _, bad := range []string{"https://example.com/icon.svg", "file:///tmp/icon.png", "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg><script/></svg>`))} {
		entry.Icon = bad
		if Validate(entry) == nil {
			t.Fatal("unsafe catalog artwork accepted")
		}
	}
}

func TestCatalogFallsBackToMirrorWhenOfficialFails(t *testing.T) {
	_, loader, _, _ := serviceFixture(t)
	entry := loader.snapshot.Entries[0]
	raw, _ := json.Marshal(entry)
	treeSHA, blobSHA := strings.Repeat("a", 40), strings.Repeat("b", 40)
	mirrorHits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/https://") {
			mirrorHits++
			if strings.Contains(r.URL.Path, "/git/blobs/") {
				_ = json.NewEncoder(w).Encode(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(raw)})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": treeSHA, "tree": []map[string]string{{"path": "plugins/example.store/plugin.json", "sha": blobSHA, "type": "blob"}}})
			return
		}
		w.WriteHeader(http.StatusForbidden) // official endpoint rate limited
	}))
	defer server.Close()

	original := ghmirror.Builtins
	ghmirror.Builtins = []ghmirror.Mirror{{ID: "test", Name: "test", Template: server.URL + "/" + ghmirror.Placeholder}}
	defer func() { ghmirror.Builtins = original }()

	c := New(t.TempDir())
	c.base = server.URL
	c.http = server.Client()
	c.SetConfig(ghmirror.Config{Mode: "auto"})
	snapshot, err := c.Load(context.Background(), true)
	if err != nil || len(snapshot.Entries) != 1 || snapshot.Revision != treeSHA {
		t.Fatalf("mirror fallback: %+v %v", snapshot, err)
	}
	if mirrorHits == 0 {
		t.Fatal("official failure did not fall back to the mirror")
	}
}

func TestCatalogOfficialOnlyReportsRateLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	c := New(t.TempDir())
	c.base = server.URL
	c.http = server.Client()
	c.SetConfig(ghmirror.Config{Mode: "official"})
	if _, err := c.Load(context.Background(), true); err == nil || !strings.Contains(err.Error(), "受限") {
		t.Fatalf("expected rate-limit error, got %v", err)
	}
}
