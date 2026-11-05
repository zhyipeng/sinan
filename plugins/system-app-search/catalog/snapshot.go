package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSnapshotBytes = 8 << 20

type diskApplication struct {
	ID    string    `json:"id"`
	Path  string    `json:"path"`
	Title string    `json:"title"`
	Names []string  `json:"names"`
	Stamp string    `json:"stamp"`
	At    time.Time `json:"at"`
}
type diskSnapshot struct {
	Version int               `json:"version"`
	Roots   []string          `json:"roots"`
	Apps    []diskApplication `json:"apps"`
}

// NewWithCache restores a disposable index. Missing/invalid caches fall back to
// normal discovery; no filesystem walk or metadata extraction happens on restore.
// An empty cacheDir keeps compatibility with hosts predating this optional field.
func NewWithCache(roots []string, cacheDir string) *Catalog {
	c := New(roots)
	if filepath.IsAbs(cacheDir) {
		c.diskPath = filepath.Join(cacheDir, "applications-v1.json")
		c.loadSnapshot()
	}
	return c
}

func (c *Catalog) snapshotRoots() []string {
	roots := make([]string, 0, len(c.roots))
	for _, root := range c.roots {
		path, err := filepath.Abs(root)
		if err == nil {
			roots = append(roots, filepath.Clean(path))
		}
	}
	return roots
}

func (c *Catalog) loadSnapshot() {
	info, err := os.Lstat(c.diskPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSnapshotBytes {
		return
	}
	f, err := os.Open(c.diskPath)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSnapshotBytes+1))
	if err != nil || len(data) > maxSnapshotBytes {
		return
	}
	var disk diskSnapshot
	if json.Unmarshal(data, &disk) != nil || disk.Version != 1 || len(disk.Apps) > 4096 || !slices.Equal(disk.Roots, c.snapshotRoots()) {
		return
	}
	cache := make(map[string]cachedApplication, len(disk.Apps))
	apps := make([]Application, 0, len(disk.Apps))
	for _, entry := range disk.Apps {
		if !validDiskApplication(entry, disk.Roots) {
			return
		}
		if _, duplicate := cache[entry.Path]; duplicate {
			return
		}
		app := Application{ID: entry.ID, Path: entry.Path, Title: entry.Title, names: entry.Names}
		cache[entry.Path] = cachedApplication{app: app, stamp: entry.Stamp, at: entry.At}
		apps = append(apps, app)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Path < apps[j].Path })
	c.apps, c.cache, c.indexed = apps, cache, true
	// Leave refreshed zero: the first query serves this snapshot and schedules validation.
}

func validDiskApplication(app diskApplication, roots []string) bool {
	if !filepath.IsAbs(app.Path) || filepath.Clean(app.Path) != app.Path || len(app.Path) > 512 || !strings.EqualFold(filepath.Ext(app.Path), ".app") || strings.ContainsRune(app.Path, '\x00') || !utf8.ValidString(app.Path) {
		return false
	}
	inside := false
	for _, root := range roots {
		rel, err := filepath.Rel(root, app.Path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			inside = true
			break
		}
	}
	hash := sha256.Sum256([]byte(app.Path))
	if !inside || app.ID != hex.EncodeToString(hash[:16]) || app.Title == "" || len(app.Title) > 256 || !utf8.ValidString(app.Title) || len(app.Names) == 0 || len(app.Names) > 16 || app.Stamp == "" || len(app.Stamp) > 8192 || app.At.IsZero() || app.At.After(time.Now().Add(time.Minute)) {
		return false
	}
	for _, name := range app.Names {
		if name == "" || len(name) > 1024 || !utf8.ValidString(name) {
			return false
		}
	}
	return true
}

// Best-effort atomic replacement. Read-only/full storage must not break search.
// Icons are intentionally excluded; only returned results request native icons.
func (c *Catalog) saveSnapshot(cache map[string]cachedApplication) {
	if c.diskPath == "" {
		return
	}
	disk := diskSnapshot{Version: 1, Roots: c.snapshotRoots(), Apps: make([]diskApplication, 0, len(cache))}
	for _, entry := range cache {
		disk.Apps = append(disk.Apps, diskApplication{ID: entry.app.ID, Path: entry.app.Path, Title: entry.app.Title, Names: entry.app.names, Stamp: entry.stamp, At: entry.at})
	}
	sort.Slice(disk.Apps, func(i, j int) bool { return disk.Apps[i].Path < disk.Apps[j].Path })
	data, err := json.Marshal(disk)
	if err != nil || len(data) > maxSnapshotBytes {
		return
	}
	if os.MkdirAll(filepath.Dir(c.diskPath), 0700) != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(c.diskPath), ".applications-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(f.Name(), c.diskPath)
}
