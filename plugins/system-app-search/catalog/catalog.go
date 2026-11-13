// Package catalog owns application discovery and matching for the trusted system plugin.
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	protocol "sinan/packages/plugin-protocol"
)

type metadata struct {
	DisplayName string `json:"CFBundleDisplayName"`
	Name        string `json:"CFBundleName"`
	BundleID    string `json:"CFBundleIdentifier"`
}
type Application struct {
	ID, Path, Title string
	names           []string
}
type cachedApplication struct {
	app   Application
	stamp string
	at    time.Time
}
type Catalog struct {
	cache      map[string]cachedApplication
	roots      []string
	mu         sync.Mutex
	apps       []Application
	indexed    bool
	refreshed  time.Time
	read       func(string) (metadata, error)
	icons      map[string]string
	launch     func(context.Context, string) error
	refreshMu  sync.Mutex
	refreshing chan struct{}
	checked    time.Time
	platform   string
	diskPath   string
}

func New(roots []string) *Catalog {
	return &Catalog{roots: append([]string(nil), roots...), read: readMetadata, icons: map[string]string{}, launch: launchApplication, platform: runtime.GOOS}
}

func DefaultRoots() []string {
	return defaultRoots(runtime.GOOS, os.Getenv, func() (string, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			if account, lookupErr := user.Current(); lookupErr == nil {
				return account.HomeDir, nil
			}
		}
		return home, err
	})
}

// Scanning and persistence never hold the search snapshot lock.
// Cancellation leaves the previous snapshot and disk cache intact.
func (c *Catalog) refresh(ctx context.Context, force bool) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.mu.Lock()
	if !force && c.indexed && time.Since(c.refreshed) <= time.Minute {
		c.mu.Unlock()
		return nil
	}
	previous := c.cache
	c.mu.Unlock()
	cache := map[string]cachedApplication{}
	apps := []Application{}
	seen := map[string]bool{}
	desktopIDs := map[string]bool{}
	for _, root := range c.roots {
		root, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		walkRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		err = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, walkErr error) error {
			// Follow an explicitly configured directory link while preserving
			// its logical entry paths and stable result IDs.
			relative, _ := filepath.Rel(walkRoot, path)
			path = filepath.Join(root, relative)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return nil
			} // Absent or unreadable roots do not invalidate other roots.
			if path != root && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if applicationEntry(c.platform, path, d.IsDir()) {
				// XDG roots are ordered: a hidden or invalid user entry masks the
				// system entry with the same desktop-file ID.
				if c.platform == "linux" {
					id := strings.ReplaceAll(relative, string(filepath.Separator), "-")
					if desktopIDs[id] {
						return nil
					}
					desktopIDs[id] = true
				}
				// Resolve entry links, but never recurse into application bundles.
				resolved, resolveErr := filepath.EvalSymlinks(path)
				info, statErr := os.Stat(path)
				if resolveErr == nil && statErr == nil && validApplicationType(c.platform, path, info) && !seen[resolved] && len(apps) < 4096 {
					seen[resolved] = true
					stamp := applicationStamp(path, resolved)
					old, exists := previous[path]
					if !force && exists && old.stamp == stamp && time.Since(old.at) < 5*time.Minute {
						apps = append(apps, old.app)
						cache[path] = old
						seen[resolved] = true
					} else if meta, err := c.read(path); err == nil {
						title := meta.DisplayName
						if title == "" {
							title = meta.Name
						}
						filename := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
						if title == "" {
							title = filename
						}
						if len(title) <= 256 && len(path) <= 512 {
							hash := sha256.Sum256([]byte(path))
							app := Application{ID: hex.EncodeToString(hash[:16]), Path: path, Title: title, names: searchNames(title, meta.Name, filename)}
							apps = append(apps, app)
							cache[path] = cachedApplication{app: app, stamp: stamp, at: time.Now()}
							seen[resolved] = true
						}
					}
				}
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				relative, _ := filepath.Rel(root, path)
				if strings.Count(relative, string(filepath.Separator)) >= 4 {
					return filepath.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Path < apps[j].Path })
	c.mu.Lock()
	icons := map[string]string{}
	for path, app := range cache {
		if old, ok := c.cache[path]; !force && ok && old.stamp == app.stamp {
			if icon, exists := c.icons[path]; exists {
				icons[path] = icon
			}
		}
	}
	c.apps, c.indexed, c.refreshed = apps, true, time.Now()
	c.icons, c.cache = icons, cache
	c.mu.Unlock()
	c.saveSnapshot(cache)
	return nil
}

// Refreshing a usable snapshot is independent of a short-lived search request.
func (c *Catalog) refreshInBackgroundLocked() {
	if c.refreshing != nil || time.Since(c.checked) < time.Minute {
		return
	}
	done := make(chan struct{})
	c.refreshing, c.checked = done, time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = c.refresh(ctx, false)
		c.mu.Lock()
		c.refreshing = nil
		close(done)
		c.mu.Unlock()
	}()
}

func match(name, query string) float64 {
	name, query = strings.ToLower(name), strings.ToLower(query)
	if name == query {
		return .8
	}
	if strings.HasPrefix(name, query) {
		return .75
	}
	if strings.Contains(name, query) {
		return .65
	}
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' || r == '.' })
	initials := ""
	for _, word := range words {
		for _, r := range word {
			initials += string(r)
			break
		}
	}
	if len(words) > 1 && strings.HasPrefix(initials, query) {
		return .55
	}
	return 0
}

func (c *Catalog) Search(ctx context.Context, p protocol.SearchParams) (protocol.SearchResult, error) {
	if p.ProviderID != "applications" {
		return protocol.SearchResult{}, errors.New("unknown provider")
	}
	if ctx.Err() != nil {
		return protocol.SearchResult{}, ctx.Err()
	}
	c.mu.Lock()
	indexed := c.indexed
	c.mu.Unlock()
	if !indexed {
		if err := c.refresh(ctx, false); err != nil {
			return protocol.SearchResult{}, err
		}
	}
	c.mu.Lock()
	if time.Since(c.refreshed) > time.Minute {
		c.refreshInBackgroundLocked()
	}
	apps := c.apps
	c.mu.Unlock()
	result := protocol.SearchResult{Items: []protocol.SearchItem{}}
	query := strings.TrimSpace(p.Query)
	if query == "" || p.Limit < 1 {
		return result, nil
	}
	for _, app := range apps {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		score := 0.0
		for _, name := range app.names {
			score = max(score, match(name, query))
		}
		if score > 0 {
			result.Items = append(result.Items, protocol.SearchItem{ID: app.ID, Title: app.Title, Subtitle: app.Path, CommandID: "launch", Input: app.ID, Score: score})
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	if len(result.Items) > min(p.Limit, 20) {
		result.Items = result.Items[:min(p.Limit, 20)]
	}
	for i := range result.Items {
		if ctx.Err() != nil {
			return protocol.SearchResult{}, ctx.Err()
		}
		for _, app := range apps {
			if app.ID != result.Items[i].ID {
				continue
			}
			c.mu.Lock()
			cached := c.cache[app.Path]
			icon, exists := c.icons[app.Path]
			c.mu.Unlock()
			if !exists {
				icon = readIcon(app.Path)
				c.mu.Lock()
				if current, ok := c.cache[app.Path]; ok && current.stamp == cached.stamp {
					c.icons[app.Path] = icon
				}
				c.mu.Unlock()
			}
			result.Items[i].Icon = icon
			break
		}
	}
	return result, nil
}

func (c *Catalog) Execute(ctx context.Context, p protocol.CommandParams) (protocol.CommandResult, error) {
	if p.CommandID == "refresh" {
		err := c.refresh(ctx, true)
		return protocol.CommandResult{Text: "应用索引已刷新"}, err
	}
	if p.CommandID != "launch" {
		return protocol.CommandResult{}, errors.New("unknown command")
	}
	c.mu.Lock()
	indexed := c.indexed
	c.mu.Unlock()
	if !indexed {
		if err := c.refresh(ctx, false); err != nil {
			return protocol.CommandResult{}, err
		}
	}
	c.mu.Lock()
	path := ""
	for _, app := range c.apps {
		if app.ID == p.Input {
			path = app.Path
			break
		}
	}
	c.mu.Unlock()
	// Input is an opaque index key. Arbitrary paths and shell fragments are rejected.
	if path == "" || ctx.Err() != nil {
		return protocol.CommandResult{}, errors.New("application unavailable")
	}
	info, err := os.Stat(path)
	if err != nil || !validApplicationType(c.platform, path, info) {
		return protocol.CommandResult{}, errors.New("application unavailable")
	}
	if _, err := c.read(path); err != nil {
		return protocol.CommandResult{}, errors.New("application unavailable")
	}
	if err := c.launch(ctx, path); err != nil {
		return protocol.CommandResult{}, errors.New("application launch failed")
	}
	return protocol.CommandResult{Dismiss: true}, nil
}

// Bundle and plist changes invalidate metadata; forced refresh also bypasses this cache.
func applicationStamp(path, target string) string {
	value := target
	for _, entry := range []string{path, filepath.Join(path, "Contents", "Info.plist"), filepath.Join(path, "Contents", "Resources")} {
		if info, err := os.Stat(entry); err == nil {
			value += fmt.Sprintf("/%d/%d", info.ModTime().UnixNano(), info.Size())
		}
	}
	return value
}
