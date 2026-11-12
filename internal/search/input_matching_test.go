package search

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"sinan/internal/inputmatch"
	"sinan/internal/kernel"
	"sinan/internal/paste"
	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

type matchingBackend struct {
	snapshot kernel.Snapshot
	calls    int
}

func (b *matchingBackend) Snapshot() kernel.Snapshot { return b.snapshot }
func (b *matchingBackend) QueryProvider(context.Context, string, protocol.SearchParams) (protocol.SearchResult, error) {
	b.calls++
	return protocol.SearchResult{}, nil
}
func queryMatches(b *matchingBackend, input string) []Item {
	var items []Item
	(&Coordinator{Backend: b}).Query(context.Background(), "test", input, func(u Update) { items = u.Items })
	return items
}
func TestDeclarativeInputMatchingKeepsRawInputAndNameSearch(t *testing.T) {
	b := &matchingBackend{snapshot: kernel.Snapshot{Plugins: []kernel.PluginInfo{{ID: "example.json", Name: "JSON Tool", Kind: "declarative", Commands: []manifest.Command{{ID: "open", Title: "JSON Tool", UIView: "editor", InputMatchers: []protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object", "array"}}}}}}}}}
	for _, input := range []string{" \n {\"big\":900719925474099312345} \t", `[1,2]`, `{"large":"` + strings.Repeat("a", 30000) + `"}`} {
		items := queryMatches(b, input)
		if len(items) != 1 || items[0].Input != input || items[0].CommandID != "open" {
			t.Fatal("matching input was lost or normalized")
		}
	}
	for _, input := range []string{"123", "true", "null", `"plain"`, "{broken", strings.Repeat("x", inputmatch.MaxInputBytes+1)} {
		if len(queryMatches(b, input)) != 0 {
			t.Fatal("non-container input matched")
		}
	}
	items := queryMatches(b, "json")
	if len(items) != 1 || items[0].Input != "" {
		t.Fatal("ordinary name search changed")
	}
	b.snapshot.Plugins[0].Disabled = true
	if len(queryMatches(b, "{}")) != 0 {
		t.Fatal("disabled plugin matched")
	}
	if b.calls != 0 {
		t.Fatal("content matching activated backend")
	}
}
func TestGenericRegexAndProviderInputBudget(t *testing.T) {
	b := &matchingBackend{snapshot: kernel.Snapshot{Plugins: []kernel.PluginInfo{{ID: "example.url", Name: "URL Tool", Kind: "process", Commands: []manifest.Command{{ID: "open", Title: "Open URL", InputMatchers: []protocol.InputMatcher{{Kind: "regex", Pattern: `^https?://\S+$`}}}}, SearchProviders: []manifest.SearchProvider{{ID: "lookup"}}}}}}
	items := queryMatches(b, "https://example.test")
	if len(items) != 1 || items[0].Input != "https://example.test" {
		t.Fatal("generic regex result not passed")
	}
	b.calls = 0
	queryMatches(b, "https://"+strings.Repeat("x", 5000))
	if b.calls != 0 {
		t.Fatal("large content was sent to legacy dynamic provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	(&Coordinator{Backend: b}).Query(ctx, "old", "https://example.test", func(Update) { t.Fatal("cancelled query emitted results") })
}

func TestPasteSearchOnlyUsesMatchingContent(t *testing.T) {
	b := &matchingBackend{snapshot: kernel.Snapshot{Plugins: []kernel.PluginInfo{
		{ID: "example.json", Name: "JSON", Kind: "declarative", Commands: []manifest.Command{{ID: "open", Title: "JSON", UIView: "main", InputMatchers: []protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object", "array"}}}}}},
		{ID: "example.keyword", Name: "JSON", Kind: "process", Commands: []manifest.Command{{ID: "open", Title: "JSON"}}, SearchProviders: []manifest.SearchProvider{{ID: "always"}}},
		{ID: "example.image", Name: "Image", Kind: "declarative", Commands: []manifest.Command{{ID: "open", Title: "Image", UIView: "main", InputMatchers: []protocol.InputMatcher{{Kind: "image"}}}}},
	}}}
	text := `{"name":"JSON"}`
	content, err := paste.Parse(protocol.PasteContent{Text: &text})
	if err != nil {
		t.Fatal(err)
	}
	var final Update
	c := Coordinator{Backend: b}
	c.QueryPaste(context.Background(), "1", "opaque", content, func(u Update) { final = u })
	if !final.Done || len(final.Items) != 1 || final.Items[0].PluginID != "example.json" || final.Items[0].PasteID != "opaque" || final.Items[0].Input != "" {
		t.Fatal("paste used name matching or exposed payload")
	}
	if b.calls != 0 {
		t.Fatal("paste activated unrelated provider")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	content, err = paste.Parse(protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(buf.Bytes())}})
	if err != nil {
		t.Fatal(err)
	}
	c.QueryPaste(context.Background(), "2", "image-input", content, func(u Update) { final = u })
	if len(final.Items) != 1 || final.Items[0].PluginID != "example.image" || final.Items[0].PasteID != "image-input" {
		t.Fatal("image reached a text-only command")
	}
	b.snapshot.Plugins[2].Disabled = true
	c.QueryPaste(context.Background(), "3", "image-input", content, func(u Update) { final = u })
	if len(final.Items) != 0 {
		t.Fatal("disabled image plugin remained visible")
	}
}
