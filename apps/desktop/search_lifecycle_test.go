package main

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/search"
)

func TestBackgroundSearchOutlivesBindingCallButKeepsWindowContext(t *testing.T) {
	window := &application.WebviewWindow{}
	h := &HostService{launcher: &launcher{window: window}}
	defer h.cancelSearch()
	caller := context.WithValue(context.Background(), application.WindowKey, window)
	request, cancelRequest := context.WithCancel(caller)
	defer cancelRequest()
	released := make(chan struct{})
	observed := make(chan context.Context, 1)
	h.startQuery(request, 1, nil, func(ctx context.Context, _ func(search.Update)) {
		<-released
		observed <- ctx
		<-ctx.Done()
	})
	// Wails cancels the binding call after StartSearch/StartPasteSearch returns.
	cancelRequest()
	close(released)
	var query context.Context
	select {
	case query = <-observed:
	case <-time.After(time.Second):
		t.Fatal("search did not start")
	}
	if query.Err() != nil {
		t.Fatal("binding completion cancelled background search")
	}
	if query.Value(application.WindowKey) != window {
		t.Fatal("search lost its window identity")
	}
	h.CancelSearch(caller, 1)
	select {
	case <-query.Done():
	case <-time.After(time.Second):
		t.Fatal("explicit cancel did not stop background search")
	}
}

func TestSearchReplacementAndStaleCancellation(t *testing.T) {
	window := &application.WebviewWindow{}
	h := &HostService{launcher: &launcher{window: window}}
	defer h.cancelSearch()
	caller := context.WithValue(context.Background(), application.WindowKey, window)
	queries := make(chan context.Context, 2)
	run := func(ctx context.Context, _ func(search.Update)) { queries <- ctx; <-ctx.Done() }
	h.startQuery(caller, 1, nil, run)
	first := <-queries
	h.startQuery(caller, 2, nil, run)
	second := <-queries
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("replacement did not cancel old query")
	}
	h.CancelSearch(caller, 1)
	if second.Err() != nil {
		t.Fatal("old effect cleanup cancelled newer search")
	}
	h.startQuery(caller, 1, nil, func(context.Context, func(search.Update)) { t.Error("stale search was started") })
	if second.Err() != nil {
		t.Fatal("stale start cancelled newer search")
	}
	h.cancelSearch()
	select {
	case <-second.Done():
	case <-time.After(time.Second):
		t.Fatal("window lifecycle did not cancel query")
	}
}
