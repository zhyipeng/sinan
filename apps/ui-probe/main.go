// A development-only, bridge-free native UI host. Nothing here is installed as
// a plugin or made available through the launcher's Wails bindings.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"sinan/internal/kernel"
	"sinan/internal/plugin/uiprocess"
	"sinan/internal/plugin/uiview"
)

//go:embed testdata/*
var fixture embed.FS

func init() { runtime.LockOSThread() }

type viewSession interface {
	Active() bool
	Resource(string) (uiview.Resource, bool)
	Message([]byte) (uiview.Action, error)
	Revoke()
	OnRevoke(func()) func()
}

type report struct {
	SeparateProcess        bool `json:"separateProcess"`
	ChildReaped            bool `json:"childReaped"`
	StartupHandshake       bool `json:"startupHandshake"`
	Ready                  bool `json:"ready"`
	BindingsAbsent         bool `json:"bindingsAbsent"`
	StorageIsolated        bool `json:"storageIsolated"`
	OwnResourceLoaded      bool `json:"ownResourceLoaded"`
	ForeignResourceBlocked bool `json:"foreignResourceBlocked"`
	NavigationBlocked      bool `json:"navigationBlocked"`
	NetworkBlocked         bool `json:"networkBlocked"`
	InvalidMessageRejected bool `json:"invalidMessageRejected"`
	CloseReceived          bool `json:"closeReceived"`
	HTTPControl            bool `json:"httpControl"`
	NoEscapeRequests       bool `json:"noEscapeRequests"`
	BudgetRevoked          bool `json:"budgetRevoked"`
	LifecycleRevoked       bool `json:"lifecycleRevoked"`
	SessionRevoked         bool `json:"sessionRevoked"`
	StaleResourceDenied    bool `json:"staleResourceDenied"`
	StaleMessageDenied     bool `json:"staleMessageDenied"`
}

func (r report) passed(flood, revoke bool) bool {
	return r.SeparateProcess && r.ChildReaped && r.StartupHandshake && r.Ready && r.BindingsAbsent && r.StorageIsolated && r.OwnResourceLoaded && r.ForeignResourceBlocked && r.NavigationBlocked && r.NetworkBlocked && r.InvalidMessageRejected && r.HTTPControl && r.NoEscapeRequests && r.CloseReceived == (!flood && !revoke) && r.BudgetRevoked == flood && r.LifecycleRevoked == revoke && r.SessionRevoked && r.StaleResourceDenied && r.StaleMessageDenied
}

func run(flood, revoke bool) bool {
	var escapes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/control" {
			escapes.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "control")
	}))
	defer server.Close()
	client := http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	control, err := client.Get(server.URL + "/control")
	if err != nil {
		return false
	}
	controlOK := control.StatusCode == http.StatusOK
	control.Body.Close()
	viewID := "normal"
	if flood {
		viewID = "flood"
	}
	if revoke {
		viewID = "revoke"
	}
	// Install fixed embedded fixtures into an isolated temporary root, so the
	// native probe exercises the same fingerprint and lifecycle owner as plugins.
	tmp, err := os.MkdirTemp("", "sinan-ui-session-probe-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)
	source := filepath.Join(tmp, "package")
	if os.MkdirAll(filepath.Join(source, "ui"), 0700) != nil {
		return false
	}
	if os.CopyFS(filepath.Join(source, "ui"), fixture) != nil {
		return false
	}
	manifest := []byte(`{"id":"example.ui-probe","name":"UI probe fixture","version":"0.1.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"uiViews":[{"id":"normal","entry":"testdata/index.html","bridgeVersion":"1"},{"id":"flood","entry":"testdata/flood.html","bridgeVersion":"1"},{"id":"revoke","entry":"testdata/revoke.html","bridgeVersion":"1"}],"commands":[{"id":"open","title":"Probe","message":"fixture"}]},"permissions":[],"activationEvents":["onCommand:open"]}`)
	if os.WriteFile(filepath.Join(source, "manifest.json"), manifest, 0600) != nil {
		return false
	}
	manager, err := kernel.New(filepath.Join(tmp, "plugins"))
	if err != nil {
		return false
	}
	defer manager.Close()
	if _, err = manager.Install(source, false); err != nil {
		return false
	}
	session, err := manager.OpenUIView(context.Background(), "example.ui-probe", viewID)
	if err != nil {
		return false
	}
	defer session.Revoke()
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{
		Executable: executable, Args: []string{"-child"}, Lifetime: 12 * time.Second,
		ProbeTarget:  server.URL + "/escape",
		ProbeDisable: func() error { _, err := manager.SetPluginEnabled("example.ui-probe", false); return err },
	})
	if err != nil {
		return false
	}
	outcome := process.Wait()
	if outcome.Err != nil {
		fmt.Fprintln(os.Stderr, outcome.Err)
		return false
	}
	var result report
	if json.Unmarshal(outcome.Report, &result) != nil {
		return false
	}
	result.SeparateProcess = process.PID() != os.Getpid()
	result.ChildReaped = true      // Wait returns only after exec.Cmd.Wait has reaped it.
	result.StartupHandshake = true // Start requires both hello and page ready.
	result.HTTPControl = controlOK
	result.NoEscapeRequests = escapes.Load() == 0
	result.SessionRevoked = !session.Active()
	_, ok := session.Resource(session.EntryURL())
	result.StaleResourceDenied = !ok
	_, messageErr := session.Message([]byte(`{"kind":"ready","version":1}`))
	result.StaleMessageDenied = messageErr != nil
	_ = json.NewEncoder(os.Stdout).Encode(result)
	return result.passed(flood, revoke)
}

func runChild() bool {
	client, err := uiprocess.Connect(os.Stdin, os.Stdout)
	if err != nil {
		return false
	}
	defer client.Close()
	init := client.Init()
	data, err := json.Marshal(struct {
		Entry string `json:"entry"`
		CSP   string `json:"csp"`
	}{init.Entry, init.CSP})
	if err != nil {
		return false
	}
	result, err := runNative(data, init.ProbeTarget, client, client.ProbeDisable)
	if err != nil {
		return false
	}
	encoded, err := json.Marshal(result)
	return err == nil && client.Finish(encoded) == nil
}

func main() {
	child := flag.Bool("child", false, "内部 UI 客体进程")
	flood := flag.Bool("flood", false, "验证消息预算耗尽后撤销桥接")
	revoke := flag.Bool("revoke", false, "验证禁用插件后撤销 UI 会话")
	flag.Parse()
	if *child {
		if !runChild() {
			os.Exit(1)
		}
		return
	}
	if *flood && *revoke || !run(*flood, *revoke) {
		fmt.Fprintln(os.Stderr, "UI 边界验证未通过或当前平台不受支持")
		os.Exit(1)
	}
}
