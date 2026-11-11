package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	protocol "sinan/packages/plugin-protocol"
	"testing"
)

type fakeEmbeddedSurface struct {
	closed int
	done   chan struct{}
}

func (f *fakeEmbeddedSurface) Show()                            {}
func (f *fakeEmbeddedSurface) Close()                           { f.closed++ }
func (f *fakeEmbeddedSurface) SetDetached()                     {}
func (f *fakeEmbeddedSurface) SetTheme(protocol.ThemeState)     {}
func (f *fakeEmbeddedSurface) SetDetachShortcut(string, uint64) {}
func (f *fakeEmbeddedSurface) Done() <-chan struct{}            { return f.done }
func TestEmbeddedReturnInvalidatesOpeningAndPreservesOtherWindows(t *testing.T) {
	first, second := &application.WebviewWindow{}, &application.WebviewWindow{}
	a, b := &fakeEmbeddedSurface{}, &fakeEmbeddedSurface{}
	host := &HostService{embedded: &embeddedViews{entries: map[*application.WebviewWindow]*embeddedEntry{first: {view: a}, second: {view: b}}, sequence: map[*application.WebviewWindow]uint64{first: 7, second: 4}}}
	host.releaseEmbedded(first)
	if a.closed != 1 || b.closed != 0 || host.embedded.entries[first] != nil || host.embedded.entries[second] == nil || host.embedded.sequence[first] != 8 {
		t.Fatal("return crossed a window boundary or did not cancel startup")
	}
	host.closeEmbeddedAll()
	if a.closed != 1 || b.closed != 1 || len(host.embedded.entries) != 0 || host.embedded.sequence[second] != 5 {
		t.Fatal("shutdown left views or pending openings")
	}
}
