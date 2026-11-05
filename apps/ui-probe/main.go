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
	"strings"
	"sync/atomic"
	"time"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uihost"
	"sinan/internal/plugin/uiprocess"
	protocol "sinan/packages/plugin-protocol"
)

//go:embed testdata/*
var fixture embed.FS

func init() { runtime.LockOSThread() }

type report struct {
	uihost.Report
	SeparateProcess     bool `json:"separateProcess"`
	ChildReaped         bool `json:"childReaped"`
	StartupHandshake    bool `json:"startupHandshake"`
	HTTPControl         bool `json:"httpControl"`
	NoEscapeRequests    bool `json:"noEscapeRequests"`
	SessionRevoked      bool `json:"sessionRevoked"`
	StaleResourceDenied bool `json:"staleResourceDenied"`
	StaleMessageDenied  bool `json:"staleMessageDenied"`
}

func (r report) passed(flood, revoke bool) bool {
	return r.EventLoopResponsive && r.Failure == "" && r.SeparateProcess && r.ChildReaped && r.StartupHandshake && r.Ready && r.BindingsAbsent && r.StorageIsolated && r.OwnResourceLoaded && r.ForeignResourceBlocked && r.NavigationBlocked && r.NetworkBlocked && r.InvalidMessageRejected && r.HTTPControl && r.NoEscapeRequests && r.CloseReceived == (!flood && !revoke) && r.BudgetRevoked == flood && r.LifecycleRevoked == revoke && r.SessionRevoked && r.StaleResourceDenied && r.StaleMessageDenied
}

func run(flood, revoke bool, fault string) bool {
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
	if fault != "" {
		viewID = "failure"
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
	if fault != "" {
		html, err := fixture.ReadFile("testdata/failure.html")
		if err != nil || os.WriteFile(filepath.Join(source, "ui", "testdata", "failure.html"), []byte(strings.ReplaceAll(string(html), "MODE", fault)), 0600) != nil {
			return false
		}
	}
	manifestData, err := json.Marshal(manifest.Manifest{
		ID: "example.ui-probe", Name: "UI probe fixture", Version: "0.1.0", APIVersion: protocol.APIVersion,
		Runtime: manifest.Runtime{Kind: "declarative"},
		Contributes: manifest.Contributions{
			Commands: []manifest.Command{{ID: "open", Title: "Probe", Message: "fixture"}},
			UIViews: []protocol.UIView{
				{ID: "normal", Entry: "testdata/index.html", BridgeVersion: protocol.UIBridgeVersion},
				{ID: "flood", Entry: "testdata/flood.html", BridgeVersion: protocol.UIBridgeVersion},
				{ID: "revoke", Entry: "testdata/revoke.html", BridgeVersion: protocol.UIBridgeVersion},
				{ID: "failure", Entry: "testdata/failure.html", BridgeVersion: protocol.UIBridgeVersion},
			},
		}, ActivationEvents: []string{"onCommand:open"},
	})
	if err != nil || os.WriteFile(filepath.Join(source, "manifest.json"), manifestData, 0600) != nil {
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
	result, err := launch(manager, viewID, server.URL+"/escape")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	result.HTTPControl = controlOK
	result.NoEscapeRequests = escapes.Load() == 0
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if fault == "script" || fault == "hang" {
		expected := "script-error"
		if fault == "hang" {
			expected = "unresponsive"
		}
		if !result.Ready || result.Failure != expected || result.CloseReceived || !result.hostPassed() {
			return false
		}
		// Keep this same kernel alive and prove another guest can still use it.
		recovered, err := launch(manager, "normal", server.URL+"/escape")
		if err != nil {
			return false
		}
		recovered.HTTPControl = controlOK
		recovered.NoEscapeRequests = escapes.Load() == 0
		_ = json.NewEncoder(os.Stdout).Encode(recovered)
		return recovered.passed(false, false)
	}
	return result.passed(flood, revoke) && (fault != "slow" || result.HeartbeatCompleted)
}

func (r report) hostPassed() bool {
	return r.EventLoopResponsive && r.SeparateProcess && r.ChildReaped && r.StartupHandshake && r.SessionRevoked && r.StaleResourceDenied && r.StaleMessageDenied && r.HTTPControl && r.NoEscapeRequests
}

func launch(manager *kernel.Manager, viewID, target string) (report, error) {
	session, err := manager.OpenUIView(context.Background(), "example.ui-probe", viewID)
	if err != nil {
		return report{}, err
	}
	defer session.Revoke()
	executable, err := os.Executable()
	if err != nil {
		return report{}, err
	}
	process, err := uiprocess.Start(context.Background(), session, uiprocess.Options{
		Executable: executable, Args: []string{"-child"}, Lifetime: 12 * time.Second,
		ProbeTarget: target, ProbeDisable: func() error { _, err := manager.SetPluginEnabled("example.ui-probe", false); return err },
	})
	if err != nil {
		return report{}, err
	}
	outcome := process.Wait()
	if outcome.Err != nil {
		return report{}, outcome.Err
	}
	var result report
	if err := json.Unmarshal(outcome.Report, &result); err != nil {
		return report{}, err
	}
	result.SeparateProcess = process.PID() != os.Getpid()
	result.ChildReaped = true      // Wait reaps the process before returning.
	result.StartupHandshake = true // Start requires hello and page ready.
	result.SessionRevoked = !session.Active()
	_, ok := session.Resource(session.EntryURL())
	result.StaleResourceDenied = !ok
	_, messageErr := session.Message([]byte(`{"kind":"ready","version":1}`))
	result.StaleMessageDenied = messageErr != nil
	return result, nil
}

func runChild() bool {
	client, err := uiprocess.Connect(os.Stdin, os.Stdout)
	if err != nil {
		return false
	}
	defer client.Close()
	init := client.Init()
	native, err := uihost.Run(uihost.Config{Entry: init.Entry, CSP: init.CSP, Target: init.ProbeTarget, Probe: true, Title: "司南 UI 边界验证", DeadlineSeconds: 8}, client, client.ProbeDisable)
	result := report{Report: native}
	if err != nil {
		return false
	}
	encoded, err := json.Marshal(result)
	return err == nil && client.Finish(encoded) == nil
}

func main() {
	child := flag.Bool("child", false, "内部 UI 客体进程")
	fault := flag.String("fault", "", "验证 script / hang / slow 页面")
	flood := flag.Bool("flood", false, "验证消息预算耗尽后撤销桥接")
	revoke := flag.Bool("revoke", false, "验证禁用插件后撤销 UI 会话")
	flag.Parse()
	if *child {
		if !runChild() {
			os.Exit(1)
		}
		return
	}
	if (*fault != "" && *fault != "script" && *fault != "hang" && *fault != "slow") || (*fault != "" && (*flood || *revoke)) || *flood && *revoke || !run(*flood, *revoke, *fault) {
		fmt.Fprintln(os.Stderr, "UI 边界验证未通过或当前平台不受支持")
		os.Exit(1)
	}
}
