// Package search aggregates static commands and independent plugin providers.
package search

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"sinan/internal/inputmatch"
	"sinan/internal/kernel"
	"sinan/internal/paste"
	protocol "sinan/packages/plugin-protocol"
)

type Backend interface {
	Snapshot() kernel.Snapshot
	QueryProvider(context.Context, string, protocol.SearchParams) (protocol.SearchResult, error)
}

type Item struct {
	Icon       string  `json:"icon,omitempty"`
	PasteID    string  `json:"pasteId,omitempty"`
	Key        string  `json:"key"`
	PluginID   string  `json:"pluginId"`
	ProviderID string  `json:"providerId"`
	CommandID  string  `json:"commandId"`
	Input      string  `json:"input"`
	Title      string  `json:"title"`
	Subtitle   string  `json:"subtitle"`
	Kind       string  `json:"kind"`
	Score      float64 `json:"score"`
}

type Update struct {
	QueryID  string   `json:"queryId"`
	Items    []Item   `json:"items"`
	Pending  int      `json:"pending"`
	Failures []string `json:"failures"`
	Done     bool     `json:"done"`
}

type Coordinator struct{ Backend Backend }
type usageRanker interface {
	UsageBoost(pluginID, commandID, input string) float64
}

var itemID = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

type provider struct {
	plugin kernel.PluginInfo
	id     string
}

func (c *Coordinator) Query(ctx context.Context, queryID, query string, emit func(Update)) {
	rawInput := query
	if len(rawInput) <= inputmatch.MaxInputBytes {
		query = strings.TrimSpace(query)
	}
	if len(rawInput) > inputmatch.MaxInputBytes || query == "" {
		if ctx.Err() == nil {
			emit(Update{QueryID: queryID, Items: []Item{}, Failures: []string{}, Done: true})
		}
		return
	}
	matcher := inputmatch.NewQuery(rawInput)
	snapshot := c.Backend.Snapshot()
	results := map[string]Item{}
	providers := []provider{}
	for _, plugin := range snapshot.Plugins {
		if ctx.Err() != nil {
			return
		}
		if plugin.Disabled {
			continue
		}
		for _, command := range plugin.Commands {
			if command.SearchOnly {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			fields := append([]string{plugin.Name, command.Title}, command.Keywords...)
			contentMatched := matcher.MatchAny(command.InputMatchers)
			nameMatched := len(query) <= 4096 && commandTextMatches(fields, query)
			if !contentMatched && !nameMatched {
				continue
			}
			score := .85
			input := ""
			if contentMatched {
				input = rawInput
				score = command.ContentMatchScore()
				if nameMatched {
					score = max(score, .85)
				}
			}
			if strings.EqualFold(command.Title, query) {
				score = 1
			}
			key := plugin.ID + "/$commands/" + command.ID
			results[key] = Item{Key: key, PluginID: plugin.ID, ProviderID: "$commands", CommandID: command.ID, Input: input, Title: command.Title, Subtitle: plugin.Name, Kind: plugin.Kind, Score: score}
		}
		for _, p := range plugin.SearchProviders {
			if len(query) > 4096 {
				continue
			}
			if utf8.RuneCountInString(query) < p.MinLength || !strings.HasPrefix(strings.ToLower(query), strings.ToLower(p.Prefix)) {
				continue
			}
			if len(providers) < 32 {
				providers = append(providers, provider{plugin: plugin, id: p.ID})
			}
		}
	}
	var mu sync.Mutex
	pending := len(providers)
	failures := []string{}
	publish := func() {
		if ctx.Err() != nil {
			return
		}
		items := make([]Item, 0, len(results))
		available := map[string]bool{}
		for _, plugin := range c.Backend.Snapshot().Plugins {
			available[plugin.ID] = !plugin.Disabled
		}
		for _, item := range results {
			if !available[item.PluginID] {
				continue
			}
			if ranker, ok := c.Backend.(usageRanker); ok {
				item.Score = min(1, item.Score+max(0, min(.025, ranker.UsageBoost(item.PluginID, item.CommandID, item.Input))))
			}
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Score != items[j].Score {
				return items[i].Score > items[j].Score
			}
			if items[i].Title != items[j].Title {
				return items[i].Title < items[j].Title
			}
			return items[i].Key < items[j].Key
		})
		if len(items) > 100 {
			items = items[:100]
		}
		emit(Update{QueryID: queryID, Items: items, Pending: pending, Failures: append([]string{}, failures...), Done: pending == 0})
	}
	publish()
	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Go(func() {
			result, err := c.Backend.QueryProvider(ctx, p.plugin.ID, protocol.SearchParams{ProviderID: p.id, Query: query, Limit: 20})
			mu.Lock()
			defer mu.Unlock()
			pending--
			if ctx.Err() != nil {
				return
			}
			available := false
			for _, current := range c.Backend.Snapshot().Plugins {
				if current.ID == p.plugin.ID && !current.Disabled {
					available = true
					break
				}
			}
			if !available {
				publish()
				return
			}
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s 查询失败", p.plugin.ID, p.id))
				publish()
				return
			}
			allowed := map[string]bool{}
			for _, command := range p.plugin.Commands {
				allowed[command.ID] = true
			}
			if len(result.Items) > 20 {
				result.Items = result.Items[:20]
			}
			seen := map[string]bool{}
			for _, item := range result.Items {
				if !itemID.MatchString(item.ID) || seen[item.ID] || !allowed[item.CommandID] || strings.TrimSpace(item.Title) == "" || len(item.Title) > 256 || len(item.Subtitle) > 512 || len(item.Input) > 64<<10 || math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
					continue
				}
				seen[item.ID] = true
				key := p.plugin.ID + "/" + p.id + "/" + item.ID
				results[key] = Item{Icon: validatedIcon(item.Icon), Key: key, PluginID: p.plugin.ID, ProviderID: p.id, CommandID: item.CommandID, Input: item.Input, Title: item.Title, Subtitle: item.Subtitle, Kind: p.plugin.Kind, Score: max(0, min(.8, item.Score))}
			}
			publish()
		})
	}
	wg.Wait()
}

// Icons cannot make network requests or load arbitrary plugin/host resources.
func validatedIcon(value string) string {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(value, prefix) || len(value) > 24<<10 {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return ""
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 128 || config.Height > 128 {
		return ""
	}
	return value
}

// QueryPaste evaluates only declared content matchers. Keyword filtering is
// applied to this result set in the Shell, never by querying unrelated commands.
func (c *Coordinator) QueryPaste(ctx context.Context, queryID, pasteID string, content *paste.Content, emit func(Update)) {
	items := make([]Item, 0)
	if content == nil {
		if ctx.Err() == nil {
			emit(Update{QueryID: queryID, Done: true, Items: []Item{}})
		}
		return
	}
	text, isText := content.Text()
	matcher := inputmatch.NewQuery(text)
	remaining := inputmatch.MaxQueryMatchers
	for _, plugin := range c.Backend.Snapshot().Plugins {
		if plugin.Disabled {
			continue
		}
		for _, command := range plugin.Commands {
			if ctx.Err() != nil {
				return
			}
			if command.SearchOnly {
				continue
			}
			matched := false
			if isText {
				matched = matcher.MatchAny(command.InputMatchers)
			} else {
				for _, rule := range command.InputMatchers {
					if remaining <= 0 {
						break
					}
					remaining--
					if rule.Kind == "image" {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
			items = append(items, Item{Key: plugin.ID + "/$commands/" + command.ID, PluginID: plugin.ID, ProviderID: "$commands", CommandID: command.ID, PasteID: pasteID, Title: command.Title, Subtitle: plugin.Name, Kind: plugin.Kind, Score: command.ContentMatchScore()})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Title != items[j].Title {
			return items[i].Title < items[j].Title
		}
		return items[i].Key < items[j].Key
	})
	available := map[string]bool{}
	for _, plugin := range c.Backend.Snapshot().Plugins {
		available[plugin.ID] = !plugin.Disabled
	}
	filtered := items[:0]
	for _, item := range items {
		if available[item.PluginID] {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	if len(items) > 100 {
		items = items[:100]
	}
	if ctx.Err() == nil {
		emit(Update{QueryID: queryID, Items: items, Done: true, Failures: []string{}})
	}
}
