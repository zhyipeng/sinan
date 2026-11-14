package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sinan/internal/plugin/manifest"
)

func TestDataCleanupRevokesOldWritersAndKeepsHostData(t *testing.T) {
	m := manager(t)
	session, _ := callableUI(t, m)
	callUI(t, session, `{"method":"storage.put","revision":0,"value":{"text":"old draft"}}`)
	config, err := m.SaveWindowPreferences(0, "Ctrl+Alt+U")
	if err != nil {
		t.Fatal(err)
	}
	m.usage.Record("example.native", "echo", "fixture")
	rows, err := m.PluginData()
	if err != nil || len(rows) != 1 || !rows[0].Installed {
		t.Fatal("bad installed metadata", err)
	}
	if err = m.ClearPluginData(rows[0].ID, rows[0].Token); err != nil {
		t.Fatal(err)
	}
	if session.Active() {
		t.Fatal("old writer remains active")
	}
	if _, err = session.Call(context.Background(), []byte(`{"method":"storage.put","revision":0,"value":{"text":"revived"}}`)); err == nil {
		t.Fatal("old writer recreated cleared data")
	}
	next := openUI(t, m, "example.native")
	next.Message([]byte(`{"kind":"ready","version":2}`))
	value := callUI(t, next, `{"method":"storage.get"}`)
	var doc struct {
		Revision uint64
		Value    json.RawMessage
	}
	json.Unmarshal(value, &doc)
	if doc.Revision != 0 || string(doc.Value) != "null" {
		t.Fatal("new session did not see empty data")
	}
	callUI(t, next, `{"method":"storage.put","revision":0,"value":{"text":"new draft"}}`)
	if err = m.ClearPluginData(rows[0].ID, rows[0].Token); err == nil {
		t.Fatal("old review cleared new data")
	}
	if !next.Active() {
		t.Fatal("failed cleanup revoked a healthy writer")
	}
	after, err := m.WindowPreferences()
	if err != nil || !reflect.DeepEqual(after, config) || m.UsageBoost("example.native", "echo", "fixture") <= 0 {
		t.Fatal("host data changed", err)
	}
	if err = m.Uninstall("example.native"); err != nil {
		t.Fatal(err)
	}
	rows, err = m.PluginData()
	if err != nil || len(rows) != 1 || rows[0].Installed || rows[0].Name != "example.native" {
		t.Fatal("orphan metadata lost", err)
	}
	if err = m.ClearPluginData(rows[0].ID, rows[0].Token); err != nil {
		t.Fatal(err)
	}
	rows, err = m.PluginData()
	if err != nil || len(rows) != 0 {
		t.Fatal("orphan retained", err)
	}
}
func TestDataCleanupFailureKeepsViewsAndOtherDocuments(t *testing.T) {
	m := manager(t)
	session, _ := callableUI(t, m)
	callUI(t, session, `{"method":"storage.put","revision":0,"value":{"text":"keep"}}`)
	m.documents.Put("example.other", 0, json.RawMessage(`{"text":"other"}`))
	rows, _ := m.PluginData()
	var token string
	for _, row := range rows {
		if row.ID == "example.native" {
			token = row.Token
		}
	}
	// Replace the reviewed document with a directory. Cleanup must not recurse.
	path := filepath.Join(m.root, ".sinan-state", "data", "example.native.json")
	backup := path + ".backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.ClearPluginData("example.native", token); err == nil {
		t.Fatal("invalid entry cleared")
	}
	if !session.Active() {
		t.Fatal("failed cleanup revoked view")
	}
	other, err := m.documents.Get("example.other")
	if err != nil || other.Revision != 1 {
		t.Fatal("other plugin data changed", err)
	}
}
func TestProtectedPluginPrivateDataCleanupKeepsPolicy(t *testing.T) {
	m := manager(t)
	src := systemSource(t, "system.settings")
	if _, err := m.Install(src, false); err != nil {
		t.Fatal(err)
	}
	m.documents.Put("system.settings", 0, json.RawMessage(`null`))
	rows, _ := m.PluginData()
	if err := m.ClearPluginData(rows[0].ID, rows[0].Token); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPluginEnabled("system.settings", false); err == nil {
		t.Fatal("cleanup relaxed disable policy")
	}
	if err := m.Uninstall("system.settings"); err == nil {
		t.Fatal("cleanup relaxed uninstall policy")
	}
	if _, err := manifest.Load(filepath.Join(m.root, "system.settings")); err != nil {
		t.Fatal("package removed", err)
	}
}

func TestEncryptedMetadataAndCleanupKeepOrdinaryData(t *testing.T) {
	m := manager(t)
	session, _ := callableUI(t, m)
	if _, err := m.documents.Put("example.native", 0, json.RawMessage(`{"ordinary":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.encryptedDocuments.Put("example.native", 0, json.RawMessage(`{"fixture":"private-fixture-value"}`)); err != nil {
		t.Fatal(err)
	}
	rows, err := m.PluginData()
	if err != nil || len(rows) != 2 {
		t.Fatal("missing encrypted metadata", err)
	}
	var encrypted PluginDataInfo
	for _, row := range rows {
		if row.Encrypted {
			encrypted = row
		}
	}
	encoded, _ := json.Marshal(rows)
	if bytes.Contains(encoded, []byte("private-fixture-value")) {
		t.Fatal("metadata exposed decrypted values")
	}
	if !strings.HasPrefix(encrypted.Token, "encrypted:") {
		t.Fatal("encrypted review token missing kind")
	}
	if err = m.ClearPluginData(encrypted.ID, encrypted.Token); err != nil {
		t.Fatal(err)
	}
	if session.Active() {
		t.Fatal("cleared encrypted namespace did not revoke views")
	}
	plain, err := m.documents.Get("example.native")
	if err != nil || plain.Revision != 1 {
		t.Fatal("clearing encrypted data removed ordinary document")
	}
	sealed, err := m.encryptedDocuments.Get("example.native")
	if err != nil || sealed.Revision != 0 {
		t.Fatal("encrypted namespace not cleared")
	}
}
