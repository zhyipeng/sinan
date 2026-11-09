package uiview

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

func sessionFixture(t *testing.T) *Session {
	t.Helper()
	b, err := LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(Binding{ViewID: "main", BridgeVersion: "1", PluginID: "example.ui", Fingerprint: strings.Repeat("a", 64)}, b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSessionRejectsUnscopedMessagesAndReplay(t *testing.T) {
	s := sessionFixture(t)
	for _, message := range []string{`{"kind":"close"}`, `{"kind":"execute","pluginId":"other.ui"}`, `{"kind":"ready","version":true}`, `{"kind":"ready","version":1,"pluginId":"other.ui"}`, `{"kind":"ready","kind":"ready","version":1}`, `{"kind":"ready","version":1} {}`, `[]`} {
		if action, err := s.Message([]byte(message)); err == nil || action != Rejected {
			t.Fatal("invalid message accepted")
		}
	}
	if action, err := s.Message([]byte(`{"kind":"ready","version":1}`)); err != nil || action != Ready {
		t.Fatal("ready rejected", err)
	}
	if _, err := s.Message([]byte(`{"kind":"ready","version":1}`)); err == nil {
		t.Fatal("ready replay accepted")
	}
	if action, err := s.Message([]byte(`{"kind":"close"}`)); err != nil || action != Close {
		t.Fatal("close rejected", err)
	}
	if s.Active() {
		t.Fatal("close did not revoke session")
	}
	if _, ok := s.Resource(s.EntryURL()); ok {
		t.Fatal("closed session read resources")
	}
	if _, err := s.Message([]byte(`{"kind":"ready","version":1}`)); err == nil {
		t.Fatal("closed session revived")
	}
}
func TestSessionBudgetRevokesAndNotifiesExactlyOnce(t *testing.T) {
	s := sessionFixture(t)
	var signals atomic.Int32
	s.OnRevoke(func() { signals.Add(1) })
	for i := 0; i < MaxSessionMessages+1; i++ {
		s.Message([]byte(strings.Repeat("x", MaxMessageBytes+1)))
	}
	if s.Active() || signals.Load() != 1 {
		t.Fatal("message budget did not revoke")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() { s.Revoke() })
	}
	wg.Wait()
	if signals.Load() != 1 {
		t.Fatal("revocation repeated")
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("revocation notification missing")
	}
}
func TestSessionResourceAndRevocationAreRaceSafe(t *testing.T) {
	s := sessionFixture(t)
	var signals atomic.Int32
	unsubscribe := s.OnRevoke(func() { signals.Add(1) })
	unsubscribe()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				s.Resource(s.EntryURL())
			}
		})
	}
	s.Revoke()
	wg.Wait()
	if signals.Load() != 0 {
		t.Fatal("removed listener ran")
	}
	s.OnRevoke(func() { signals.Add(1) })
	if signals.Load() != 1 {
		t.Fatal("late listener missed revoked state")
	}
	if _, ok := s.Resource(s.EntryURL()); ok {
		t.Fatal("resource read after revoke returned bytes")
	}
}

func TestSessionBindingCannotClaimAnotherOwner(t *testing.T) {
	b, err := LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range []Binding{{ViewID: "main", BridgeVersion: "1", PluginID: "example.other", Fingerprint: strings.Repeat("a", 64)}, {ViewID: "main", BridgeVersion: "1", PluginID: "example.ui", Fingerprint: "invalid"}, {ViewID: "main", BridgeVersion: "1", PluginID: "", Fingerprint: strings.Repeat("a", 64)}} {
		if _, err := NewSession(binding, b); err == nil {
			t.Fatal("invalid session binding accepted")
		}
	}
}

func TestSessionRejectsMissingOrIncompatibleUIBinding(t *testing.T) {
	s := sessionFixture(t)
	b, err := LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Binding){
		func(b *Binding) { b.ViewID = "" }, func(b *Binding) { b.ViewID = "../main" }, func(b *Binding) { b.BridgeVersion = "" }, func(b *Binding) { b.BridgeVersion = "99" },
	} {
		binding := s.Binding()
		mutate(&binding)
		if _, err := NewSession(binding, b); err == nil {
			t.Fatal("invalid UI binding accepted")
		}
	}
}
