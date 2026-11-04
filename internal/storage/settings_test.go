package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSettingsNamespacesPersistenceAndRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := OpenSettings(path)
	values := map[string][]string{"directories": {"/fixture"}}
	first, err := s.Update("system.test", 0, values)
	if err != nil || first.Revision != 1 {
		t.Fatal("first update failed", err)
	}
	values["directories"][0] = "/changed"
	first.Values["directories"][0] = "/changed"
	if s.Get("system.test").Values["directories"][0] != "/fixture" {
		t.Fatal("mutable state escaped")
	}
	if _, err = s.Update("system.test", 0, map[string][]string{}); err == nil {
		t.Fatal("stale revision accepted")
	}
	if OpenSettings(path).Get("system.test").Revision != 1 || len(s.Get("other.test").Values) != 0 {
		t.Fatal("namespace persistence failure")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("settings file not private")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if _, err := s.Update("system.test", 1, map[string][]string{}); err == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent updates overwrote one another")
	}
}
func TestSettingsMigrationAndCorruptionPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"plugins":{"system.test":{"revision":2,"values":{"directories":["/fixture"]}}}}`), 0600)
	s := OpenSettings(path)
	if s.Error() != nil || s.Get("system.test").Revision != 2 {
		t.Fatal("legacy migration failed")
	}
	if _, err := s.Update("system.test", 2, map[string][]string{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var doc settingsDocument
	json.Unmarshal(data, &doc)
	if doc.Version != 1 {
		t.Fatal("migration not persisted")
	}
	for _, value := range []string{`invalid`, `{"version":99,"plugins":{}}`, `{"version":1,"plugins":{}} {}`} {
		os.WriteFile(path, []byte(value), 0600)
		s = OpenSettings(path)
		if s.Error() == nil {
			t.Fatal("bad settings accepted")
		}
		if _, err := s.Update("system.test", 0, map[string][]string{}); err == nil {
			t.Fatal("corrupt settings overwritten")
		}
		data, _ := os.ReadFile(path)
		if string(data) != value {
			t.Fatal("original file lost")
		}
	}
}
func TestSettingsWriteFailureRollsBack(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	s := OpenSettings(filepath.Join(parent, "settings.json"))
	os.WriteFile(parent, []byte("fixture"), 0600)
	if _, err := s.Update("system.test", 0, map[string][]string{}); err == nil || s.Get("system.test").Revision != 0 {
		t.Fatal("failed write changed revision")
	}
}
