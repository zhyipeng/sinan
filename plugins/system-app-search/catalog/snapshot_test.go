package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

func snapshotFixture(t *testing.T) (*Catalog, string, protocol.SearchParams) {
	t.Helper()
	c, root := fixture(t)
	c.diskPath = filepath.Join(t.TempDir(), "applications-v1.json")
	p := protocol.SearchParams{ProviderID: "applications", Query: "vsc", Limit: 20}
	if result, err := c.Search(context.Background(), p); err != nil || len(result.Items) != 1 {
		t.Fatal("initial discovery failed", err)
	}
	if _, err := os.Stat(c.diskPath); err != nil {
		t.Fatal("snapshot not persisted", err)
	}
	return c, root, p
}

func TestWarmSnapshotServesDuringValidationAndRejectsRemovedLaunch(t *testing.T) {
	c, root, p := snapshotFixture(t)
	restored := NewWithCache(c.roots, filepath.Dir(c.diskPath))
	restored.platform = "darwin"
	if !restored.indexed || len(restored.apps) != 2 {
		t.Fatal("snapshot not restored")
	}
	if err := os.RemoveAll(filepath.Join(root, "Visual Studio Code.app")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Fresh.app"), 0700); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); waitForRefresh(t, restored) })
	restored.read = func(path string) (metadata, error) {
		if filepath.Base(path) == "Broken.app" {
			close(entered)
			<-release
		}
		return c.read(path)
	}
	results := make(chan protocol.SearchResult, 1)
	go func() { result, _ := restored.Search(context.Background(), p); results <- result }()
	var result protocol.SearchResult
	select {
	case result = <-results:
	case <-time.After(3 * time.Second):
		t.Fatal("warm query waited for validation")
	}
	if len(result.Items) != 1 || result.Items[0].ID != c.apps[0].ID && result.Items[0].ID != c.apps[1].ID {
		t.Fatal("cached result missing")
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("validation was not scheduled")
	}
	// Even with a blocked metadata read, another query can access the old snapshot.
	again, err := restored.Search(context.Background(), p)
	if err != nil || len(again.Items) != 1 {
		t.Fatal("background scan blocked subsequent search", err)
	}
	launched := false
	restored.launch = func(context.Context, string) error { launched = true; return nil }
	if _, err := restored.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: result.Items[0].ID}); err == nil || launched {
		t.Fatal("removed cached application launched")
	}
	unblock()
	waitForRefresh(t, restored)
	removed, err := restored.Search(context.Background(), p)
	if err != nil || len(removed.Items) != 0 {
		t.Fatal("removed item survived validation", err)
	}
	fresh, err := restored.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "fresh", Limit: 20})
	if err != nil || len(fresh.Items) != 1 {
		t.Fatal("new application not discovered", err)
	}
	if next := NewWithCache(c.roots, filepath.Dir(c.diskPath)); len(next.apps) != 2 || next.cache[filepath.Join(root, "Fresh.app")].app.Title == "" {
		t.Fatal("validated snapshot was not persisted")
	}
}

func TestInvalidSnapshotsAndChangedRootsFallBackToDiscovery(t *testing.T) {
	c, root, p := snapshotFixture(t)
	data, err := os.ReadFile(c.diskPath)
	if err != nil {
		t.Fatal(err)
	}
	var valid diskSnapshot
	if err = json.Unmarshal(data, &valid); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*diskSnapshot)
	}{
		{"version", func(s *diskSnapshot) { s.Version = 999 }},
		{"roots", func(s *diskSnapshot) { s.Roots = []string{filepath.Join(root, "other")} }},
		{"outside-root", func(s *diskSnapshot) { s.Apps[0].Path = "/Outside.app" }},
		{"identity", func(s *diskSnapshot) { s.Apps[0].ID = "invalid" }},
		{"duplicate", func(s *diskSnapshot) { s.Apps = append(s.Apps, s.Apps[0]) }},
		{"future", func(s *diskSnapshot) { s.Apps[0].At = time.Now().Add(time.Hour) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var disk diskSnapshot
			json.Unmarshal(data, &disk)
			tc.change(&disk)
			bad, _ := json.Marshal(disk)
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "applications-v1.json"), bad, 0600)
			next := NewWithCache(c.roots, dir)
			if next.indexed {
				t.Fatal("invalid cache accepted")
			}
			next.read = c.read
			next.platform = "darwin"
			if result, err := next.Search(context.Background(), p); err != nil || len(result.Items) != 1 {
				t.Fatal("cache failure broke discovery", err)
			}
		})
	}
	for _, bad := range [][]byte{[]byte("{broken"), bytes.Repeat([]byte("x"), maxSnapshotBytes+1)} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "applications-v1.json"), bad, 0600)
		if NewWithCache(c.roots, dir).indexed {
			t.Fatal("malformed or oversized cache accepted")
		}
	}
	if NewWithCache([]string{root}, filepath.Dir(c.diskPath)).indexed {
		t.Fatal("changed root set reused old cache")
	}
}

func TestManualRefreshCancellationAndCacheWriteFailure(t *testing.T) {
	c, root, _ := snapshotFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "New.app"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "refresh"}); err != nil {
		t.Fatal(err)
	}
	if NewWithCache(c.roots, filepath.Dir(c.diskPath)).cache[filepath.Join(root, "New.app")].app.Title == "" {
		t.Fatal("manual refresh did not update cache")
	}
	info, err := os.Stat(c.diskPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("snapshot permissions", err)
	}
	before, _ := os.ReadFile(c.diskPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Execute(ctx, protocol.CommandParams{CommandID: "refresh"}); err == nil {
		t.Fatal("cancelled refresh accepted")
	}
	after, _ := os.ReadFile(c.diskPath)
	if !bytes.Equal(before, after) {
		t.Fatal("cancelled scan overwrote cache")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, []byte("fixture"), 0600)
	c.diskPath = filepath.Join(blocker, "applications-v1.json")
	if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "refresh"}); err != nil {
		t.Fatal("cache write error broke usable index", err)
	}
}
