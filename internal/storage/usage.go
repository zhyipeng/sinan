// Package storage provides host-owned, bounded local state.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const maxUsageEntries = 4096

type usageEntry struct {
	Count    uint32 `json:"count"`
	LastUsed int64  `json:"lastUsed"`
}
type Usage struct {
	mu      sync.Mutex
	path    string
	entries map[string]usageEntry
}

func actionKey(plugin, command, input string) string {
	// JSON framing avoids ambiguous separators; raw input is never persisted.
	data, _ := json.Marshal([]string{plugin, command, input})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func OpenUsage(path string) *Usage {
	u := &Usage{path: path, entries: map[string]usageEntry{}}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if info, err := f.Stat(); err == nil && info.Size() <= 1<<20 {
			var entries map[string]usageEntry
			if json.NewDecoder(f).Decode(&entries) == nil && len(entries) <= maxUsageEntries {
				for key, entry := range entries {
					_, err := hex.DecodeString(key)
					if err == nil && len(key) == 64 && entry.Count > 0 && entry.LastUsed > 0 && entry.LastUsed <= time.Now().Unix()+60 {
						u.entries[key] = entry
					}
				}
			}
		}
	}
	return u
}
func (u *Usage) Record(plugin, command, input string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	key := actionKey(plugin, command, input)
	entry := u.entries[key]
	if entry.Count < 1000000 {
		entry.Count++
	}
	entry.LastUsed = time.Now().Unix()
	u.entries[key] = entry
	if len(u.entries) > maxUsageEntries {
		keys := make([]string, 0, len(u.entries))
		for key := range u.entries {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := u.entries[keys[i]], u.entries[keys[j]]
			if a.LastUsed != b.LastUsed {
				return a.LastUsed < b.LastUsed
			}
			return keys[i] < keys[j]
		})
		delete(u.entries, keys[0])
	}
	// Ranking is best effort: a disk failure must not turn a successful action into a failure.
	_ = u.saveLocked()
}
func (u *Usage) Boost(plugin, command, input string) float64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	entry := u.entries[actionKey(plugin, command, input)]
	if entry.Count == 0 {
		return 0
	}
	age := max(0, time.Since(time.Unix(entry.LastUsed, 0)).Hours()/24)
	frequency := min(1, math.Log2(float64(entry.Count)+1)/5)
	return .025 * (.7*frequency + .3*math.Exp(-age/14))
}
func (u *Usage) Clear() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	old := u.entries
	u.entries = map[string]usageEntry{}
	if err := u.saveLocked(); err != nil {
		u.entries = old
		return err
	}
	return nil
}
func (u *Usage) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(u.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(u.entries)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(u.path), ".usage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), u.path)
}
