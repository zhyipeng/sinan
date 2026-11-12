package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sinan/internal/kernel"
	"sinan/internal/plugin/distribution"
	"strings"
	"testing"
	"time"
)

type fixtureLoader struct{ snapshot Snapshot }

func (l *fixtureLoader) Load(context.Context, bool) (Snapshot, error) { return l.snapshot, nil }

type fixtureStager struct {
	directory string
	cleaned   int
}

func (f *fixtureStager) Stage(context.Context, string, distribution.Artifact) (string, func(), error) {
	return f.directory, func() { f.cleaned++ }, nil
}
func serviceFixture(t *testing.T) (*Service, *fixtureLoader, *fixtureStager, *kernel.Manager) {
	t.Helper()
	source := t.TempDir()
	data := `{"id":"example.store","name":"商店测试","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"hello","title":"测试","message":"hello"}]},"permissions":[],"activationEvents":["onCommand:hello"]}`
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	repo := "https://github.com/example/plugin"
	release := Release{SchemaVersion: 1, PluginID: "example.store", Name: "商店测试", Version: "0.1.0", Repository: "example/plugin", Tag: "v0.1.0", Commit: strings.Repeat("a", 40), ReleaseID: 1, MinHostVersion: "0.1.0", Compatibility: Compatibility{API: "1", UI: "2"}, Assets: []Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, File: "example.store-0.1.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip", SHA256: strings.Repeat("a", 64), Size: 100, ID: 7}}}
	release.Assets[0].URL = "https://github.com/example/plugin/releases/download/v0.1.0/" + release.Assets[0].File
	entry := Entry{SchemaVersion: 1, ID: "example.store", Name: "商店测试", Summary: "test", Publisher: Publisher{Kind: "community"}, Source: Source{Repository: &repo, Path: "."}, Status: "published", Releases: []Release{release}}
	loader := &fixtureLoader{Snapshot{Entries: []Entry{entry}, Revision: "reviewed", UpdatedAt: time.Now().UnixMilli()}}
	stager := &fixtureStager{directory: source}
	s := NewService(m, t.TempDir())
	s.Loader = loader
	s.Fetcher = stager
	t.Cleanup(s.Close)
	return s, loader, stager, m
}
func TestStoreInstallUsesReviewedPreviewAndSingleUseTicket(t *testing.T) {
	s, _, stager, m := serviceFixture(t)
	preview, err := s.Prepare(context.Background(), "example.store", "0.1.0", "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), preview.Ticket, false); err != nil {
		t.Fatal(err)
	}
	if len(m.Snapshot().Plugins) != 1 || stager.cleaned != 1 {
		t.Fatal("package was not installed or cleaned")
	}
	if _, err = s.Apply(context.Background(), preview.Ticket, false); err == nil {
		t.Fatal("ticket reused")
	}
}
func TestWithdrawnExpiredOrChangedBaselineCannotInstall(t *testing.T) {
	for _, kind := range []string{"withdrawn", "expired", "baseline"} {
		t.Run(kind, func(t *testing.T) {
			s, loader, stager, m := serviceFixture(t)
			preview, err := s.Prepare(context.Background(), "example.store", "0.1.0", "reviewed")
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "withdrawn":
				loader.snapshot.Entries[0].Status = "withdrawn"
			case "expired":
				p := s.pending[preview.Ticket]
				p.expires = time.Now().Add(-time.Minute)
				s.pending[preview.Ticket] = p
			case "baseline":
				if _, err = m.Install(stager.directory, false); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.Apply(context.Background(), preview.Ticket, false); err == nil {
				t.Fatal("invalidated install proceeded")
			}
			if stager.cleaned != 1 {
				t.Fatal("invalidated stage not cleaned")
			}
		})
	}
}
func TestPrepareRefusesStaleCatalogAndArchiveIdentityMismatch(t *testing.T) {
	s, loader, stager, _ := serviceFixture(t)
	loader.snapshot.Stale = true
	if _, err := s.Prepare(context.Background(), "example.store", "0.1.0", "reviewed"); err == nil {
		t.Fatal("stale catalog allowed")
	}
	loader.snapshot.Stale = false
	if _, err := s.Prepare(context.Background(), "example.store", "0.1.0", "old"); err == nil {
		t.Fatal("stale revision allowed")
	}
	path := filepath.Join(stager.directory, "manifest.json")
	data, _ := os.ReadFile(path)
	var mf map[string]any
	json.Unmarshal(data, &mf)
	mf["id"] = "example.other"
	data, _ = json.Marshal(mf)
	os.WriteFile(path, data, 0600)
	if _, err := s.Prepare(context.Background(), "example.store", "0.1.0", "reviewed"); err == nil {
		t.Fatal("archive identity mismatch allowed")
	}
}
func TestNativeInstallStillRequiresExplicitTrust(t *testing.T) {
	s, _, stager, m := serviceFixture(t)
	path := filepath.Join(stager.directory, "manifest.json")
	data, _ := os.ReadFile(path)
	var mf map[string]any
	json.Unmarshal(data, &mf)
	mf["runtime"] = map[string]string{"kind": "process", "entry": "plugin"}
	mf["contributes"] = map[string]any{"commands": []any{map[string]string{"id": "hello", "title": "测试"}}}
	data, _ = json.Marshal(mf)
	os.WriteFile(path, data, 0600)
	os.WriteFile(filepath.Join(stager.directory, "plugin"), []byte("not executed"), 0700)
	preview, err := s.Prepare(context.Background(), "example.store", "0.1.0", "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(context.Background(), preview.Ticket, false); err == nil || len(m.Snapshot().Plugins) != 0 {
		t.Fatal("native trust was bypassed")
	}
}

func TestStoreManualUpdatePreservesInstalledState(t *testing.T) {
	s, loader, stager, m := serviceFixture(t)
	if _, err := m.Install(stager.directory, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPluginEnabled("example.store", false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stager.directory, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), `"0.1.0"`, `"0.2.0"`))
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loader.snapshot.Entries[0].Releases[0].Version = "0.2.0"
	preview, err := s.Prepare(context.Background(), "example.store", "0.2.0", "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	if preview.InstalledVersion != "0.1.0" {
		t.Fatal("missing installed version")
	}
	if _, err = s.Apply(context.Background(), preview.Ticket, false); err != nil {
		t.Fatal(err)
	}
	installed := m.Snapshot().Plugins[0]
	if installed.Version != "0.2.0" || !installed.Disabled {
		t.Fatal("update failed or changed enablement")
	}
}
