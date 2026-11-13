package transport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	process, err := Start(path, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(process.Stop)
	rpc, end := context.WithTimeout(context.Background(), 5*time.Second)
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
	process.Stop()
	if process.Alive() {
		t.Fatal("process remained alive after stop")
	}
}
