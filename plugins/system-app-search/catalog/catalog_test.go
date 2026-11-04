package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

func fixture(t *testing.T) (*Catalog, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"Visual Studio Code.app", "工具/计算器.app", ".Hidden.app", "Broken.app", "Visual Studio Code.app/Contents/Helper.app"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "Visual Studio Code.app"), filepath.Join(root, "ZAlias.app")); err != nil {
		t.Fatal(err)
	}
	c := New([]string{root, root, filepath.Join(root, "absent")})
	c.read = func(path string) (metadata, error) {
		if filepath.Base(path) == "Broken.app" {
			return metadata{}, errors.New("broken")
		}
		return metadata{Name: filepath.Base(path)}, nil
	}
	return c, root
}

func TestDiscoveryMatchingAndStableIdentity(t *testing.T) {
	c, root := fixture(t)
	result, err := c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "vsc", Limit: 20})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("matching: %v %d", err, len(result.Items))
	}
	id := result.Items[0].ID
	if len(c.apps) != 2 {
		t.Fatalf("indexed nested, hidden, duplicate or broken bundle: %d", len(c.apps))
	}
	for _, query := range []string{"VISUAL", "studio code", "计算"} {
		result, err = c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: query, Limit: 1})
		if err != nil || len(result.Items) != 1 {
			t.Fatalf("query %s: %v", query, err)
		}
	}
	if _, err = c.Execute(context.Background(), protocol.CommandParams{CommandID: "refresh"}); err != nil {
		t.Fatal(err)
	}
	result, _ = c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "vsc", Limit: 20})
	if result.Items[0].ID != id || result.Items[0].Subtitle != filepath.Join(root, "Visual Studio Code.app") {
		t.Fatal("unstable identity")
	}
}

func TestLaunchOnlyIndexedExistingApplication(t *testing.T) {
	c, root := fixture(t)
	launched := ""
	c.launch = func(_ context.Context, path string) error { launched = path; return nil }
	result, _ := c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "vsc", Limit: 20})
	for _, input := range []string{"/Applications/Other.app", "$(touch /tmp/sinan-should-not-exist)", "../escape"} {
		if _, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: input}); err == nil {
			t.Fatal("accepted arbitrary launch input")
		}
	}
	res, err := c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: result.Items[0].ID})
	if err != nil || !res.Dismiss || launched != filepath.Join(root, "Visual Studio Code.app") {
		t.Fatal("indexed launch failed")
	}
	launched = ""
	os.RemoveAll(filepath.Join(root, "Visual Studio Code.app"))
	if _, err = c.Execute(context.Background(), protocol.CommandParams{CommandID: "launch", Input: result.Items[0].ID}); err == nil || launched != "" {
		t.Fatal("launched removed application")
	}
}

func TestCancellationPreservesSnapshotAndRefresh(t *testing.T) {
	c, root := fixture(t)
	c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "vsc", Limit: 20})
	before := len(c.apps)
	ctx, cancel := context.WithCancel(context.Background())
	c.read = func(string) (metadata, error) { cancel(); return metadata{}, nil }
	if _, err := c.Execute(ctx, protocol.CommandParams{CommandID: "refresh"}); err == nil || len(c.apps) != before {
		t.Fatal("cancelled refresh replaced snapshot")
	}
	c.read = func(path string) (metadata, error) { return metadata{Name: filepath.Base(path)}, nil }
	os.MkdirAll(filepath.Join(root, "New.app"), 0700)
	c.refreshed = time.Time{}
	result, err := c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "New", Limit: 20})
	if err != nil || len(result.Items) != 1 {
		t.Fatal("expired index not refreshed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			c.Search(context.Background(), protocol.SearchParams{ProviderID: "applications", Query: "new", Limit: 20})
		})
	}
	wg.Wait()
}
