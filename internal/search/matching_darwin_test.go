//go:build darwin && cgo

package search

import (
	"context"
	"testing"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

type staticCommandBackend struct{ snapshot kernel.Snapshot }

func (b *staticCommandBackend) Snapshot() kernel.Snapshot { return b.snapshot }
func (b *staticCommandBackend) QueryProvider(context.Context, string, protocol.SearchParams) (protocol.SearchResult, error) {
	panic("static command search must not activate a provider")
}
func staticQuery(b *staticCommandBackend, query string) []Item {
	var items []Item
	c := Coordinator{Backend: b}
	c.Query(context.Background(), "fixture", query, func(update Update) { items = update.Items })
	return items
}

func TestPluginCommandPinyinMatchesPluginTitleAndKeyword(t *testing.T) {
	b := &staticCommandBackend{snapshot: kernel.Snapshot{Plugins: []kernel.PluginInfo{
		{ID: "system.settings", Name: "设置", Commands: []manifest.Command{{ID: "open", Title: "设置"}}},
		{ID: "system.plugin-manager", Name: "插件管理", Commands: []manifest.Command{{ID: "open", Title: "插件管理"}}},
		{ID: "example.title", Name: "Tools", Commands: []manifest.Command{{ID: "about", Title: "关于"}}},
		{ID: "example.keyword", Name: "Editor", Commands: []manifest.Command{{ID: "open", Title: "Open", Keywords: []string{"窗口独立"}}}},
		{ID: "example.hidden", Name: "设置", Commands: []manifest.Command{{ID: "open", Title: "设置", SearchOnly: true}}},
		{ID: "example.disabled", Name: "设置", Disabled: true, Commands: []manifest.Command{{ID: "open", Title: "设置"}}},
	}}}
	for query, id := range map[string]string{
		"shezhi": "system.settings", "SZ": "system.settings", "she zhi": "system.settings",
		"chajianguanli": "system.plugin-manager", "cjgl": "system.plugin-manager",
		"guanyu": "example.title", "gy": "example.title", "ckdl": "example.keyword",
		"设置": "system.settings", "关于": "example.title", "editor": "example.keyword",
	} {
		items := staticQuery(b, query)
		if len(items) != 1 || items[0].PluginID != id || items[0].ProviderID != "$commands" || items[0].Input != "" {
			t.Fatalf("query %q did not match expected static command", query)
		}
	}
}

func TestCommandPinyinKeepsExactRankingAndFollowsChangedMetadata(t *testing.T) {
	b := &staticCommandBackend{snapshot: kernel.Snapshot{Plugins: []kernel.PluginInfo{
		{ID: "example.chinese", Name: "Tools", Commands: []manifest.Command{{ID: "open", Title: "设置"}}},
		{ID: "example.literal", Name: "Literal", Commands: []manifest.Command{{ID: "open", Title: "shezhi"}}},
	}}}
	items := staticQuery(b, "shezhi")
	if len(items) != 2 || items[0].PluginID != "example.literal" || items[0].Score <= items[1].Score {
		t.Fatal("exact title ranking changed")
	}
	b.snapshot.Plugins = b.snapshot.Plugins[:1]
	b.snapshot.Plugins[0].Commands[0].Title = "插件管理"
	if len(staticQuery(b, "shezhi")) != 0 {
		t.Fatal("old metadata remained searchable")
	}
	if len(staticQuery(b, "cjgl")) != 1 {
		t.Fatal("changed title not indexed")
	}
}

func TestCommandAliasCacheIsBounded(t *testing.T) {
	aliasCache.Lock()
	aliasCache.values = make(map[string][]string, 2048)
	for i := range 2048 {
		aliasCache.values[string(rune(i+1))] = []string{"fixture"}
	}
	aliasCache.Unlock()
	if !commandTextMatches([]string{"设置"}, "shezhi") {
		t.Fatal("fresh alias missed")
	}
	aliasCache.Lock()
	defer aliasCache.Unlock()
	if len(aliasCache.values) > 2048 {
		t.Fatal("unbounded command alias retention")
	}
}
