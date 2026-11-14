package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sinan/internal/llm"
	"sinan/internal/plugin/manifest"
)

func TestSharedLLMCapabilityIsolationAndCancellation(t *testing.T) {
	m := manager(t)
	session, src := callableUI(t, m)
	if _, e := session.Call(context.Background(), []byte(`{"method":"llm.config"}`)); e == nil {
		t.Fatal("LLM capability granted without declaration")
	}
	mf, e := manifest.Load(src)
	if e != nil {
		t.Fatal(e)
	}
	mf.Version = "0.1.1"
	mf.Contributes.UIViews[0].Permissions = []string{"llm"}
	writeManifest(t, src, mf)
	if _, e = m.Upgrade(src, true); e != nil {
		t.Fatal(e)
	}
	session = openUI(t, m, "example.native")
	session.Message([]byte(`{"kind":"ready","version":2}`))
	other := openUI(t, m, "example.native")
	other.Message([]byte(`{"kind":"ready","version":2}`))
	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-value" {
			t.Error("missing host-owned credential")
		}
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer server.Close()
	_, e = m.SaveLLMProfile(llm.Edit{Name: "shared", BaseURL: server.URL, Model: "fixture", APIKey: "fixture-value"})
	if e != nil {
		t.Fatal(e)
	}
	meta := callUI(t, session, `{"method":"llm.config"}`)
	if strings.Contains(string(meta), "fixture-value") || strings.Contains(string(meta), server.URL) {
		t.Fatal("plugin saw private host configuration")
	}
	start := callUI(t, session, `{"method":"llm.start","value":{"systemPrompt":"Translate","input":"hello"}}`)
	var job struct {
		ID string `json:"id"`
	}
	json.Unmarshal(start, &job)
	if job.ID == "" {
		t.Fatal("no asynchronous job")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	poll := fmt.Sprintf(`{"method":"llm.poll","input":%q}`, job.ID)
	if _, e = other.Call(context.Background(), []byte(poll)); e == nil {
		t.Fatal("another session read a shared job")
	}
	if _, e = session.Call(context.Background(), []byte(`{"method":"storage.encrypted.get"}`)); e == nil {
		t.Fatal("LLM declaration exposed encrypted storage")
	}
	for _, input := range []string{`{"method":"llm.config","pluginId":"other"}`, `{"method":"llm.start","value":{"input":"hello","apiKey":"fixture"}}`} {
		if _, e = session.Call(context.Background(), []byte(input)); e == nil {
			t.Fatal("unrecognized LLM authority accepted")
		}
	}
	session.Revoke()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("revocation did not cancel shared request")
	}
	if _, e = other.Call(context.Background(), []byte(poll)); e == nil {
		t.Fatal("revoked job remained available")
	}
	secondStart := callUI(t, other, `{"method":"llm.start","value":{"systemPrompt":"Translate","input":"again"}}`)
	var second struct {
		ID string `json:"id"`
	}
	json.Unmarshal(secondStart, &second)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("second request did not start")
	}
	settings, err := m.LLMSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.SaveLLMPreferences(llm.Preferences{Revision: settings.Revision, DefaultProfile: settings.DefaultProfile, Overrides: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("configuration change did not cancel running request")
	}
	if _, err = other.Call(context.Background(), []byte(fmt.Sprintf(`{"method":"llm.poll","input":%q}`, second.ID))); err == nil {
		t.Fatal("old result survived configuration update")
	}

}
func TestGlobalLLMChangesCancelJobsAndDefaultsArePrivate(t *testing.T) {
	m := manager(t)
	first, e := m.SaveLLMProfile(llm.Edit{Name: "one", BaseURL: "https://one.example/v1", Model: "m", APIKey: "fixture-value"})
	if e != nil {
		t.Fatal(e)
	}
	second, e := m.SaveLLMProfile(llm.Edit{Revision: first.Revision, Name: "two", BaseURL: "https://two.example/v1", Model: "m"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = m.SaveLLMPreferences(llm.Preferences{Revision: second.Revision, DefaultProfile: first.DefaultProfile, Overrides: map[string]string{"example.native": second.Profiles[1].ID}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.encryptedDocuments.Get("$llm"); e == nil {
		t.Fatal("plugin storage namespace exposed shared credentials")
	}
	profiles, e := m.PluginData()
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range profiles {
		if p.ID == "$llm" {
			t.Fatal("shared credentials exposed as plugin document")
		}
	}
	m.Close()
	if _, e = m.LLMSettings(); e == nil {
		t.Fatal("closed runtime retained LLM settings authority")
	}
}
