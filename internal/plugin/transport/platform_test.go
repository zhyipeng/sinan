package transport

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
)

// Exercise the real process entry, sanitized environment and stdio handshake
// on every target, without relying on POSIX scripts or opening a desktop UI.
func TestPlatformNativeProcessHandshake(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "fixture with spaces", "example.transport")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	entry := "plugin"
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	path := filepath.Join(directory, entry)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "../../kernel/testdata/plugin")
	if err := cmd.Run(); err != nil {
		t.Fatal("could not build native fixture", err)
	}
	if _, err := manifest.Executable(directory, manifest.Manifest{Runtime: manifest.Runtime{Kind: "process", Entry: entry}}); err != nil {
		t.Fatal("native entry rejected", err)
	}
	hostStarted, hostCancelled := make(chan struct{}), make(chan struct{})
	var hostCalls atomic.Int32
	process, err := StartWithHost(path, directory, func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		hostCalls.Add(1)
		var p protocol.ClipboardParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		if p.Sequence == 100 {
			close(hostStarted)
			<-ctx.Done()
			close(hostCancelled)
			return nil, ctx.Err()
		}
		if p.Sequence == 99 {
			return nil, errors.New("fixture denied")
		}
		if method == "clipboard.write" {
			if len(p.Content.Data) != 2<<20 {
				return nil, errors.New("bad payload")
			}
			return protocol.ClipboardResult{}, nil
		}
		return protocol.ClipboardResult{Content: protocol.ClipboardContent{Kind: "image", Data: make([]byte, 2<<20)}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(process.Stop)
	rpc, end := context.WithTimeout(context.Background(), 10*time.Second)
	defer end()
	var initialized protocol.InitializeResult
	if err := process.Call(rpc, "initialize", protocol.InitializeParams{APIVersion: "1", PluginID: "example.transport"}, &initialized); err != nil {
		t.Fatal("handshake failed", err)
	}
	if initialized.APIVersion != "1" || initialized.PluginID != "example.transport" {
		t.Fatal("wrong handshake identity")
	}
	var result protocol.CommandResult
	if err := process.Call(rpc, "command.execute", protocol.CommandParams{CommandID: "echo", Input: "跨平台"}, &result); err != nil || result.Text != "跨平台" {
		t.Fatal("stdio roundtrip failed", err)
	}
	for _, input := range []string{"read", "write", "denied"} {
		err := process.Call(rpc, "command.execute", protocol.CommandParams{CommandID: "host-clipboard", Input: input}, &result)
		if (err != nil) != (input == "denied") {
			t.Fatalf("host RPC %s failed: %v", input, err)
		}
		if err == nil && input == "read" && result.Text != "2097152" {
			t.Fatal("large response lost")
		}
	}
	if hostCalls.Load() != 3 {
		t.Fatal("missing reverse calls")
	}
	// A rejected reverse call must leave both directions operational.
	if err := process.Call(rpc, "command.execute", protocol.CommandParams{CommandID: "echo", Input: "after"}, &result); err != nil || result.Text != "after" {
		t.Fatal("host error broke stream", err)
	}
	waiting := make(chan error, 1)
	go func() {
		var r protocol.CommandResult
		waiting <- process.Call(rpc, "command.execute", protocol.CommandParams{CommandID: "host-clipboard", Input: "wait"}, &r)
	}()
	select {
	case <-hostStarted:
	case <-rpc.Done():
		t.Fatal("reverse request did not start")
	}
	process.Stop()
	select {
	case <-hostCancelled:
	case <-rpc.Done():
		t.Fatal("process stop did not cancel reverse request")
	}
	if err := <-waiting; err == nil {
		t.Fatal("stopped reverse request succeeded")
	}
	if process.Alive() {
		t.Fatal("process remained alive after stop")
	}
}
