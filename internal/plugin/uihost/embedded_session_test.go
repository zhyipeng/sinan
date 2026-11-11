package uihost

import (
	"context"
	"sinan/internal/plugin/uiview"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedResourceBudgetsRevokeInsteadOfFallingBack(t *testing.T) {
	for _, test := range []struct{ size, count int }{{1, 256}, {uiview.MaxFileBytes, 16}} {
		bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: make([]byte, test.size)}}, "example.embedded", "index.html")
		if err != nil {
			t.Fatal(err)
		}
		session, err := uiview.NewSession(uiview.Binding{PluginID: "example.embedded", ViewID: "main", BridgeVersion: "2", Fingerprint: strings.Repeat("ab", 32)}, bundle)
		if err != nil {
			t.Fatal(err)
		}
		adapter := &embeddedSession{Session: session}
		for i := 0; i < test.count; i++ {
			if _, ok := adapter.Resource(session.EntryURL()); !ok {
				t.Fatalf("premature denial at %d", i)
			}
		}
		if _, ok := adapter.Resource(session.EntryURL()); ok || session.Active() {
			t.Fatal("aggregate resource budget was bypassed")
		}
	}
}
