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
	"os/exec"
	"os/user"
	"path/filepath"
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
	cache     map[string]cachedApplication
	roots     []string
	mu        sync.Mutex
	apps      []Application
	indexed   bool
	refreshed time.Time
	read      func(string) (metadata, error)
	icons     map[string]string
	launch    func(context.Context, string) error
}

func New(roots []string) *Catalog {
	return &Catalog{roots: roots, read: readMetadata, icons: map[string]string{}, launch: func(ctx context.Context, path string) error {
		// Pass a single absolute application path, never a shell command or a bundle ID.
		return exec.CommandContext(ctx, "/usr/bin/open", "-a", path).Run()
	}}
}

func DefaultRoots() []string {
	roots := []string{"/Applications", "/System/Applications", "/System/Library/CoreServices/Applications"}
	home, err := os.UserHomeDir()
	if err != nil {
		if account, lookupErr := user.Current(); lookupErr == nil {
			home = account.HomeDir
		}
	}
	if home != "" {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	return roots
}

// refreshLocked builds a replacement snapshot. Cancellation leaves the old index intact.
func (c *Catalog) refreshLocked(ctx context.Context, force bool) error {
	cache := map[string]cachedApplication{}
	icons := map[string]string{}
	apps := []Application{}
	seen := map[string]bool{}
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
			if strings.EqualFold(filepath.Ext(d.Name()), ".app") {
				// Follow only .app entry symlinks (Safari uses one); never walk their targets.
				resolved, resolveErr := filepath.EvalSymlinks(path)
				info, statErr := os.Stat(path)
				if resolveErr == nil && statErr == nil && info.IsDir() && !seen[resolved] && len(apps) < 4096 {
					seen[resolved] = true
					stamp := applicationStamp(path, resolved)
					old, exists := c.cache[path]
					if !force && exists && old.stamp == stamp && time.Since(old.at) < 5*time.Minute {
						apps = append(apps, old.app)
						cache[path] = old
						if icon, ok := c.icons[path]; ok {
							icons[path] = icon
						}
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
	c.apps, c.indexed, c.refreshed = apps, true, time.Now()
	c.icons, c.cache = icons, cache
	return nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return protocol.SearchResult{}, ctx.Err()
	}
	if !c.indexed || time.Since(c.refreshed) > time.Minute {
		if err := c.refreshLocked(ctx, false); err != nil {
			return protocol.SearchResult{}, err
		}
	}
	result := protocol.SearchResult{Items: []protocol.SearchItem{}}
	query := strings.TrimSpace(p.Query)
	if query == "" || p.Limit < 1 {
		return result, nil
	}
	for _, app := range c.apps {
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
		for _, app := range c.apps {
			if app.ID != result.Items[i].ID {
				continue
			}
			icon, exists := c.icons[app.Path]
			if !exists {
				icon = readIcon(app.Path)
				c.icons[app.Path] = icon
			}
			result.Items[i].Icon = icon
			break
		}
	}
	return result, nil
}

func (c *Catalog) Execute(ctx context.Context, p protocol.CommandParams) (protocol.CommandResult, error) {
	c.mu.Lock()
	if p.CommandID == "refresh" {
		err := c.refreshLocked(ctx, true)
		c.mu.Unlock()
		return protocol.CommandResult{Text: "应用索引已刷新"}, err
	}
	if p.CommandID != "launch" {
		c.mu.Unlock()
		return protocol.CommandResult{}, errors.New("unknown command")
	}
	if !c.indexed {
		if err := c.refreshLocked(ctx, false); err != nil {
			c.mu.Unlock()
			return protocol.CommandResult{}, err
		}
	}
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
	if err != nil || !info.IsDir() {
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
