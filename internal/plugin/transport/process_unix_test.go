//go:build darwin || linux

package transport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func inheritedStderrProcess(t *testing.T) *Process {
	t.Helper()
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	quoted := "'" + strings.ReplaceAll(pidFile, "'", "'\\''") + "'"
	script := filepath.Join(root, "plugin")
	data := "#!/bin/sh\n/bin/sleep 30 >/dev/null &\nprintf '%s' \"$!\" > " + quoted + "\nwait\n"
	if err := os.WriteFile(script, []byte(data), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := Start(script, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, _ := os.ReadFile(pidFile)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 {
			if child, err := os.FindProcess(pid); err == nil {
				_ = child.Kill()
			}
		}
		p.Stop()
	})
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		if data, _ := os.ReadFile(pidFile); len(data) > 0 {
			return p
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant fixture did not start")
	return nil
}
func TestStopDoesNotWaitForDescendantStderr(t *testing.T) {
	p := inheritedStderrProcess(t)
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for inherited stderr")
	}
}
func TestCommandTimeoutDoesNotWaitForDescendantStderr(t *testing.T) {
	p := inheritedStderrProcess(t)
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		var result any
		done <- p.Call(ctx, "fixture", nil, &result)
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("timeout was not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("command deadline blocked on Stop")
	}
}
