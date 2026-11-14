package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"sinan/internal/storage"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func newFixture(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	return New(storage.OpenEncryptedDocuments(filepath.Join(root, "data"), filepath.Join(root, "keys", "master")))
}
func add(t *testing.T, s *Service, revision uint64, name, host, key string) Snapshot {
	t.Helper()
	v, e := s.Save(Edit{Revision: revision, Name: name, BaseURL: "https://" + host + "/v1", Model: name, APIKey: key})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSharedProfilesRouteByExplicitOverrideAndDefault(t *testing.T) {
	s := newFixture(t)
	a := add(t, s, 0, "fast", "a.example", "fixture-value-a")
	b := add(t, s, a.Revision, "quality", "b.example", "fixture-value-b")
	idA, idB := b.Profiles[0].ID, b.Profiles[1].ID
	preferences, e := s.Preferences(Preferences{Revision: b.Revision, DefaultProfile: idA, Overrides: map[string]string{"example.two": idB}})
	if e != nil {
		t.Fatal(e)
	}
	meta, _ := json.Marshal(preferences)
	if strings.Contains(string(meta), "fixture-value") {
		t.Fatal("settings snapshot exposed credentials")
	}
	choices, e := s.Choices("example.two")
	if e != nil || choices.DefaultProfile != idB || len(choices.Profiles) != 2 {
		t.Fatal("invalid choices")
	}
	meta, _ = json.Marshal(choices)
	for _, field := range []string{"baseURL", "model", "apiKey", "fixture-value"} {
		if strings.Contains(string(meta), field) {
			t.Fatal("plugin choices exposed private configuration")
		}
	}
	var requests []string
	s.client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "fixture-value") {
			t.Fatal("credential appeared in prompt")
		}
		requests = append(requests, r.URL.Host+"|"+r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"result"}}]}`)), Header: http.Header{}}, nil
	})
	for _, tc := range []struct{ plugin, profile, want string }{{"example.one", "", "a.example|Bearer fixture-value-a"}, {"example.two", "", "b.example|Bearer fixture-value-b"}, {"example.two", idA, "a.example|Bearer fixture-value-a"}, {"example.one", idB, "b.example|Bearer fixture-value-b"}} {
		text, err := s.Complete(context.Background(), tc.plugin, Request{ProfileID: tc.profile, SystemPrompt: "Translate", Input: "hello"})
		if err != nil || text != "result" || requests[len(requests)-1] != tc.want {
			t.Fatal("shared routing failed")
		}
	}
	if _, err := s.Complete(context.Background(), "example.one", Request{ProfileID: "missing", Input: "hello"}); err == nil {
		t.Fatal("missing explicit profile silently fell back")
	}
}
func TestProfileUpdatesProtectKeysAndRevisions(t *testing.T) {
	s := newFixture(t)
	first := add(t, s, 0, "first", "a.example", "fixture-value")
	id := first.Profiles[0].ID
	if _, e := s.Save(Edit{Revision: 0, ID: id, Name: "renamed", BaseURL: "https://a.example/v1", Model: "m"}); e == nil {
		t.Fatal("stale edit accepted")
	}
	renamed, e := s.Save(Edit{Revision: first.Revision, ID: id, Name: "renamed", BaseURL: "https://a.example/v1", Model: "m"})
	if e != nil || !renamed.Profiles[0].HasKey {
		t.Fatal("blank edit lost stored key")
	}
	if _, e = s.Save(Edit{Revision: renamed.Revision, ID: id, Name: "renamed", BaseURL: "https://other.example/v1", Model: "m"}); e == nil {
		t.Fatal("key reused across changed address")
	}
	cleared, e := s.Save(Edit{Revision: renamed.Revision, ID: id, Name: "renamed", BaseURL: "https://other.example/v1", Model: "m", ClearKey: true})
	if e != nil || cleared.Profiles[0].HasKey {
		t.Fatal("key not cleared")
	}
	deleted, e := s.Delete(id, cleared.Revision)
	if e != nil || len(deleted.Profiles) != 0 || deleted.DefaultProfile != "" {
		t.Fatal("profile deletion failed")
	}
}
func TestTransportRejectsRedirectAndDoesNotEchoErrors(t *testing.T) {
	s := newFixture(t)
	add(t, s, 0, "test", "a.example", "fixture-value")
	for _, status := range []int{302, 401, 403, 429, 500} {
		count := 0
		s.client.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) {
			count++
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("fixture-value")), Header: http.Header{"Location": []string{"https://other.example/collect"}}}, nil
		})
		if _, e := s.Complete(context.Background(), "example.one", Request{Input: "hello"}); e == nil || strings.Contains(e.Error(), "fixture-value") || count != 1 {
			t.Fatal("unsafe error or redirect handling")
		}
	}
	for _, body := range []string{`{"choices":[]}`, `{"choices":[{"message":{"content":"fixture-value"}}]}`, strings.Repeat("x", (256<<10)+1)} {
		s.client.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})
		if _, e := s.Complete(context.Background(), "example.one", Request{Input: "hello"}); e == nil || strings.Contains(e.Error(), "fixture-value") {
			t.Fatal("invalid response accepted or exposed")
		}
	}
}
func TestEndpointAndReservedVault(t *testing.T) {
	for _, address := range []string{"https://example.com/v1", "http://127.0.0.1:1234/v1", "http://[::1]:1234/v1"} {
		if _, e := Endpoint(Profile{Name: "test", Model: "m", BaseURL: address}); e != nil {
			t.Fatal(e)
		}
	}
	for _, address := range []string{"http://remote.example/v1", "https://u:p@example.com/v1", "https://example.com/v1?key=fixture", "file:///tmp/a"} {
		if _, e := Endpoint(Profile{Name: "test", Model: "m", BaseURL: address}); e == nil {
			t.Fatal("invalid endpoint allowed")
		}
	}
	s := newFixture(t)
	add(t, s, 0, "test", "example.com", "")
	if _, e := s.vault.Get("$llm"); e == nil {
		t.Fatal("plugin namespace can read host secret")
	}
	rows, e := s.vault.List()
	if e != nil || len(rows) != 0 {
		t.Fatal("host secrets listed as plugin data")
	}
}
