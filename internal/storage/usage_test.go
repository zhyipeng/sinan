package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUsagePersistsBoundedBoostWithoutRawInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "usage.json")
	u := OpenUsage(path)
	if u.Boost("system.test", "launch", "opaque-target") != 0 {
		t.Fatal("new action has history")
	}
	u.Record("system.test", "launch", "opaque-target")
	first := u.Boost("system.test", "launch", "opaque-target")
	for i := 0; i < 20; i++ {
		u.Record("system.test", "launch", "opaque-target")
	}
	if boost := u.Boost("system.test", "launch", "opaque-target"); boost <= first || boost > .025 {
		t.Fatal("frequency weight out of bounds")
	}
	if OpenUsage(path).Boost("system.test", "launch", "opaque-target") == 0 {
		t.Fatal("history lost on reload")
	}
	if u.Boost("other.test", "launch", "opaque-target") != 0 {
		t.Fatal("history leaked across plugins")
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.Contains(data, []byte("opaque-target")) || bytes.Contains(data, []byte("system.test")) {
		t.Fatal("raw action data persisted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("history permissions are not private")
	}
	if err := u.Clear(); err != nil || OpenUsage(path).Boost("system.test", "launch", "opaque-target") != 0 {
		t.Fatal("clear did not persist")
	}
}
func TestUsageCorruptionDecayAndConcurrentRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	os.WriteFile(path, []byte("invalid JSON"), 0600)
	u := OpenUsage(path)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() { u.Record("system.test", "launch", "id") })
	}
	wg.Wait()
	key := actionKey("system.test", "launch", "id")
	if u.entries[key].Count != 8 {
		t.Fatal("concurrent updates lost")
	}
	recent := u.Boost("system.test", "launch", "id")
	entry := u.entries[key]
	entry.LastUsed = time.Now().Add(-90 * 24 * time.Hour).Unix()
	u.entries[key] = entry
	if u.Boost("system.test", "launch", "id") >= recent {
		t.Fatal("recency does not decay")
	}
	entries := map[string]usageEntry{}
	for i := 0; i < maxUsageEntries; i++ {
		entries[actionKey("test", "command", string(rune(i)))] = usageEntry{Count: 1, LastUsed: 1}
	}
	data, _ := json.Marshal(entries)
	os.WriteFile(path, data, 0600)
	u = OpenUsage(path)
	u.Record("system.test", "launch", "new")
	if len(u.entries) != maxUsageEntries || u.Boost("system.test", "launch", "new") == 0 {
		t.Fatal("history is not bounded")
	}
}

func TestPersistenceFailureDoesNotLoseInMemorySuccess(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	os.WriteFile(parent, []byte("fixture"), 0600)
	u := OpenUsage(filepath.Join(parent, "usage.json"))
	u.Record("system.test", "launch", "id")
	if u.Boost("system.test", "launch", "id") == 0 {
		t.Fatal("disk failure lost successful usage")
	}
	if u.Clear() == nil || u.Boost("system.test", "launch", "id") == 0 {
		t.Fatal("failed clear did not restore state")
	}
}
