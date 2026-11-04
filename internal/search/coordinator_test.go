package search

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

type fakeBackend struct {
	failSlow bool
	slow     chan struct{}
	started  chan string
	mu       sync.Mutex
	calls    int
}

func (f *fakeBackend) Snapshot() kernel.Snapshot {
	plugins := []kernel.PluginInfo{}
	for _, id := range []string{"example.fast", "example.slow"} {
		plugins = append(plugins, kernel.PluginInfo{ID: id, Name: id, Kind: "process", Commands: []manifest.Command{{ID: "echo", Title: "Echo"}}, SearchProviders: []manifest.SearchProvider{{ID: "demo", Prefix: "q", MinLength: 2}}})
	}
	return kernel.Snapshot{Plugins: plugins}
}
func (f *fakeBackend) QueryProvider(ctx context.Context, id string, p protocol.SearchParams) (protocol.SearchResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.started != nil {
		f.started <- id
	}
	if id == "example.slow" {
		select {
		case <-f.slow:
		case <-ctx.Done():
			return protocol.SearchResult{}, ctx.Err()
		}
	}
	if id == "example.slow" && f.failSlow {
		return protocol.SearchResult{}, errors.New("test failure")
	}
	return protocol.SearchResult{Items: []protocol.SearchItem{{ID: "same", Title: id, CommandID: "echo", Score: 1e9}, {ID: "spoof", Title: "invalid command", CommandID: "unregistered", Score: 1}}}, nil
}
func TestIncrementalParallelProvidersAndScopedActions(t *testing.T) {
	f := &fakeBackend{slow: make(chan struct{}), started: make(chan string, 2)}
	c := Coordinator{Backend: f}
	updates := make(chan Update, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Query(context.Background(), "1", "query", func(u Update) { updates <- u })
	}()
	initial := <-updates
	if initial.Pending != 2 || len(initial.Items) != 0 {
		t.Fatal(initial)
	}
	<-f.started
	<-f.started
	select {
	case fast := <-updates:
		if fast.Pending != 1 || len(fast.Items) != 1 || fast.Items[0].PluginID != "example.fast" {
			t.Fatal(fast)
		}
	case <-time.After(time.Second):
		t.Fatal("fast provider waited for slow provider")
	}
	close(f.slow)
	<-done
	final := <-updates
	if !final.Done || len(final.Items) != 2 || final.Items[0].Key == final.Items[1].Key {
		t.Fatal(final)
	}
	for _, item := range final.Items {
		if item.Score > .8 || !strings.HasPrefix(item.Key, item.PluginID+"/") || item.CommandID != "echo" {
			t.Fatal(item)
		}
	}
}
func TestCancellationDropsLateResults(t *testing.T) {
	f := &fakeBackend{slow: make(chan struct{}), started: make(chan string, 2)}
	c := Coordinator{Backend: f}
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan Update, 8)
	done := make(chan struct{})
	go func() { defer close(done); c.Query(ctx, "old", "query", func(u Update) { updates <- u }) }()
	<-updates
	<-f.started
	<-f.started
	// Consume the fast provider result before cancelling the slow query.
	<-updates
	cancel()
	close(f.slow)
	<-done
	if len(updates) != 0 {
		t.Fatal("late cancelled result published")
	}
}
func TestStaticCommandsDoNotActivateProviders(t *testing.T) {
	f := &fakeBackend{slow: make(chan struct{})}
	c := Coordinator{Backend: f}
	var final Update
	c.Query(context.Background(), "static", "Echo", func(u Update) { final = u })
	if !final.Done || len(final.Items) != 2 || f.calls != 0 {
		t.Fatal(final, f.calls)
	}
}

func TestProviderFailureDoesNotRemoveOtherResults(t *testing.T) {
	slow := make(chan struct{})
	close(slow)
	f := &fakeBackend{slow: slow, failSlow: true}
	c := Coordinator{Backend: f}
	var final Update
	c.Query(context.Background(), "failure", "query", func(u Update) { final = u })
	if !final.Done || len(final.Items) != 1 || final.Items[0].PluginID != "example.fast" || len(final.Failures) != 1 {
		t.Fatal(final)
	}
}

func TestIconRejectsExternalResourcesAndOversizedImages(t *testing.T) {
	for _, value := range []string{"https://example.test/icon.png", "file:///tmp/icon.png", "data:image/svg+xml,<svg/>", "data:image/png;base64,invalid", strings.Repeat("x", 25000)} {
		if validatedIcon(value) != "" {
			t.Fatal("invalid icon accepted")
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	if validatedIcon(valid) != valid {
		t.Fatal("valid icon rejected")
	}
	data.Reset()
	png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 129, 1)))
	if validatedIcon("data:image/png;base64,"+base64.StdEncoding.EncodeToString(data.Bytes())) != "" {
		t.Fatal("oversized icon accepted")
	}
}

type searchOnlyBackend struct{ fakeBackend }

func (f *searchOnlyBackend) Snapshot() kernel.Snapshot {
	return kernel.Snapshot{Plugins: []kernel.PluginInfo{{ID: "system.test", Name: "Test", Kind: "process", Commands: []manifest.Command{{ID: "launch", Title: "Launch", SearchOnly: true}}}}}
}
func TestSearchOnlyCommandsHaveNoEmptyStaticAction(t *testing.T) {
	c := Coordinator{Backend: &searchOnlyBackend{}}
	c.Query(context.Background(), "1", "Launch", func(u Update) {
		if len(u.Items) != 0 || !u.Done {
			t.Fatal("search-only command exposed without target")
		}
	})
}
