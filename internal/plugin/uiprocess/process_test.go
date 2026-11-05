package uiprocess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"sinan/internal/plugin/uiview"
)

var guest string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sinan-ui-guest-test-")
	if err != nil {
		os.Exit(1)
	}
	guest = filepath.Join(dir, "guest")
	cmd := exec.Command("go", "build", "-o", guest, "./testdata/guest")
	if cmd.Run() != nil {
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func session(t *testing.T) *uiview.Session {
	t.Helper()
	bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}, "large.js": {Data: make([]byte, uiview.MaxFileBytes)}}, "example.ui", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	s, err := uiview.NewSession(uiview.Binding{ViewID: "main", BridgeVersion: "1", PluginID: "example.ui", Fingerprint: strings.Repeat("ab", 32)}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Revoke)
	return s
}

func options(mode string) Options {
	return Options{Executable: guest, Args: []string{mode}, StartupTimeout: time.Second, Lifetime: 5 * time.Second, ShutdownGrace: 200 * time.Millisecond}
}

func TestPrivatePipeRoundTrip(t *testing.T) {
	t.Setenv("SINAN_TEST_PARENT_MARKER", "fixture-parent-only")
	s := session(t)
	p, err := Start(context.Background(), s, options("normal"))
	if err != nil {
		t.Fatal(err)
	}
	r := p.Wait()
	if r.Err != nil || string(r.Report) != `{"ok":true}` {
		t.Fatalf("round trip failed: %v", r.Err)
	}
	if s.Active() || p.PID() == os.Getpid() || !p.cmd.ProcessState.Exited() {
		t.Fatal("guest not revoked and reaped")
	}
	p.Stop()
	p.Stop()
}

func TestStartupFailuresRevokeAndReap(t *testing.T) {
	for _, mode := range []string{"crash", "silent", "bad", "version", "duplicate", "oversize", "hello-only"} {
		t.Run(mode, func(t *testing.T) {
			s := session(t)
			opts := options(mode)
			opts.StartupTimeout = 250 * time.Millisecond
			start := time.Now()
			if p, err := Start(context.Background(), s, opts); err == nil || p != nil {
				t.Fatal("invalid guest accepted")
			}
			if s.Active() || time.Since(start) > 3*time.Second {
				t.Fatal("startup did not revoke promptly")
			}
		})
	}
	t.Run("missing executable", func(t *testing.T) {
		s := session(t)
		opts := options("normal")
		opts.Executable = filepath.Join(t.TempDir(), "missing")
		if _, err := Start(context.Background(), s, opts); err == nil || s.Active() {
			t.Fatal("failed startup retained authority")
		}
	})
}

func TestTerminationAndFaultIsolation(t *testing.T) {
	for _, mode := range []string{"cancel", "lifetime", "revoke", "ready-crash"} {
		t.Run(mode, func(t *testing.T) {
			s := session(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := options("hang")
			if mode == "ready-crash" {
				opts.Args = []string{mode}
			}
			if mode == "lifetime" {
				opts.Lifetime = 300 * time.Millisecond
			}
			p, err := Start(ctx, s, opts)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "cancel":
				cancel()
			case "revoke":
				s.Revoke()
			}
			if r := p.Wait(); r.Err == nil || s.Active() || p.cmd.ProcessState == nil {
				t.Fatal("failed guest not reaped")
			}
			other := session(t)
			healthy, err := Start(context.Background(), other, options("normal"))
			if err != nil {
				t.Fatal(err)
			}
			if healthy.Wait().Err != nil {
				t.Fatal("other session affected")
			}
		})
	}
}

func TestCooperativeRevokeAndConcurrentStop(t *testing.T) {
	s := session(t)
	p, err := Start(context.Background(), s, options("wait-revoke"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(p.Stop)
	}
	wg.Wait()
	r := p.Wait()
	if r.Err != nil || string(r.Report) != `{"revoked":true}` {
		t.Fatalf("cooperative shutdown: %v", r.Err)
	}
}

func TestPipeBudgets(t *testing.T) {
	for _, mode := range []string{"request-budget", "resource-budget"} {
		t.Run(mode, func(t *testing.T) {
			s := session(t)
			p, err := Start(context.Background(), s, options(mode))
			if err != nil {
				t.Fatal(err)
			}
			result := p.Wait()
			if result.Err == nil || s.Active() || p.cmd.ProcessState == nil {
				t.Fatal("budget exhaustion retained guest")
			}
		})
	}
}

func TestStrictEnvelope(t *testing.T) {
	for _, frame := range []string{
		`{"id":1,"kind":"hello","version":1,"Kind":"hello"}`,
		`{"id":1,"kind":"hello","version":1,"id":1}`,
		`{"id":1,"kind":"hello","version":1,"owner":"other"}`,
		`{"id":1,"kind":"hello","version":1} {}`,
	} {
		var req request
		if decode([]byte(frame), &req) == nil {
			t.Fatal("noncanonical envelope accepted")
		}
	}
}
