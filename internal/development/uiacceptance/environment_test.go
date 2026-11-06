package uiacceptance

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sinan/internal/search"
	"sort"
	"testing"
)

func TestPrepareAndCleanup(t *testing.T) {
	packages := filepath.Join("..", "..", "..", "plugins")
	before, err := os.ReadFile(filepath.Join(packages, "system-plugin-manager", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := Prepare(packages)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	var ids []string
	for _, plugin := range env.Manager.Snapshot().Plugins {
		ids = append(ids, plugin.ID)
	}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, []string{PluginID, "system.plugin-manager", "system.settings"}) {
		t.Fatal("unexpected acceptance plugins")
	}
	var result search.Update
	(&search.Coordinator{Backend: env.Manager}).Query(context.Background(), "acceptance", "窗口验收", func(update search.Update) { result = update })
	if !result.Done || len(result.Items) != 1 || result.Items[0].PluginID != PluginID || result.Items[0].CommandID != "open" {
		t.Fatal("acceptance view command not searchable")
	}
	commandSession, err := env.Manager.OpenUICommand(context.Background(), PluginID, "open")
	if err != nil || commandSession.Binding().ViewID != "input" {
		t.Fatal("acceptance command did not resolve its view")
	}
	session, err := env.Manager.OpenUIView(context.Background(), PluginID, "input")
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"index.html", "main.js", "style.css"} {
		if _, ok := session.Resource("sinan-plugin://" + PluginID + "/" + resource); !ok {
			t.Fatal("acceptance resource unavailable")
		}
	}
	env.Close()
	env.Close()
	if _, err = os.Stat(env.Directory); !os.IsNotExist(err) {
		t.Fatal("temporary environment not removed")
	}
	if session.Active() {
		t.Fatal("cleanup retained session")
	}
	after, err := os.ReadFile(filepath.Join(packages, "system-plugin-manager", "manifest.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("source package changed")
	}
}

func TestPrepareFailureRemovesTemporaryDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if env, err := Prepare(filepath.Join(tmp, "missing")); err == nil || env != nil {
		t.Fatal("missing system packages accepted")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed preparation left temporary files")
	}
}
