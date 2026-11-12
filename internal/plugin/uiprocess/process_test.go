package uiprocess

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	return Options{Executable: guest, Args: []string{mode}, StartupTimeout: 3 * time.Second, Lifetime: 5 * time.Second, ShutdownGrace: 200 * time.Millisecond}
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
	for _, mode := range []string{"crash", "silent", "bad", "version", "duplicate", "oversize", "hello-only", "unsolicited-focused"} {
		t.Run(mode, func(t *testing.T) {
			s := session(t)
			opts := options(mode)
			opts.StartupTimeout = 250 * time.Millisecond
			if mode == "unsolicited-focused" {
				opts.StartupTimeout = 3 * time.Second
			}
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
				opts.Lifetime = 4 * time.Second
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

func TestFocusAcknowledgementAndRevocation(t *testing.T) {
	s := session(t)
	opts := options("wait-revoke")
	opts.Lifetime = 10 * time.Second
	p, err := Start(context.Background(), s, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	for range 260 {
		if err = p.Focus(); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Active() {
		t.Fatal("solicited focus acknowledgements consumed the page request budget")
	}
	s.Revoke()
	if p.Focus() == nil {
		t.Fatal("revoked window focused")
	}
	if p.Wait().Err != nil {
		t.Fatal("cooperative focus guest failed")
	}
}

func TestFocusTimeoutDoesNotKillHealthyGuest(t *testing.T) {
	s := session(t)
	opts := options("delay-focus")
	opts.Lifetime = 10 * time.Second
	p, err := Start(context.Background(), s, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	if p.Focus() == nil || !s.Active() {
		t.Fatal("late focus did not time out while preserving guest")
	}
	if p.Focus() == nil {
		t.Fatal("second pending control accepted")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		waiting := len(p.focusPending) > 0
		p.mu.Unlock()
		if !waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("late acknowledgement not drained")
		}
		time.Sleep(time.Millisecond)
	}
	if p.Focus() != nil {
		t.Fatal("late acknowledgement poisoned further controls")
	}
	if !s.Active() {
		t.Fatal("focus timeout killed guest")
	}
}

func TestRevocationInterruptsPendingFocus(t *testing.T) {
	s := session(t)
	p, err := Start(context.Background(), s, options("hang"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	result := make(chan error, 1)
	go func() { result <- p.Focus() }()
	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		pending := len(p.focusPending) > 0
		p.mu.Unlock()
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("focus did not start")
		}
		time.Sleep(time.Millisecond)
	}
	s.Revoke()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("revocation accepted pending focus")
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not interrupt focus")
	}
	if p.Wait().Err == nil {
		t.Fatal("uncooperative guest not reaped")
	}
	if p.Focus() == nil {
		t.Fatal("closed guest revived")
	}
}

func TestWindowActionRequiresPanelRole(t *testing.T) {
	s := session(t)
	opts := options("back")
	called := make(chan struct{}, 1)
	opts.WindowAction = func(string) bool { called <- struct{}{}; return true }
	p, err := Start(context.Background(), s, opts)
	if err != nil {
		t.Fatal(err)
	}
	result := p.Wait()
	select {
	case <-called:
		t.Fatal("non-panel invoked window action")
	default:
	}
	if result.Err == nil || s.Active() {
		t.Fatal("non-panel window action was not rejected")
	}
}

func TestLiveDetachShortcutUpdatesKeepSessionAndRejectStaleRevision(t *testing.T) {
	s := session(t)
	opts := options("wait-shortcut")
	opts.DetachShortcut = "Shift+Cmd+D"
	p, err := Start(context.Background(), s, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	pid := p.PID()
	if err := p.SetDetachShortcut("Ctrl+Alt+F19", 2); err != nil {
		t.Fatal(err)
	}
	if err := p.SetDetachShortcut("Shift+Cmd+D", 1); err != nil {
		t.Fatal(err)
	}
	if err := p.Focus(); err != nil {
		t.Fatal(err)
	}
	if !s.Active() || pid != p.PID() {
		t.Fatal("updating shortcut replaced session")
	}
	p.Stop()
	result := p.Wait()
	if result.Err != nil || string(result.Report) != `{"shortcut":"Ctrl+Alt+F19"}` {
		t.Fatal("renderer missed live preference update", result.Err)
	}
}

func TestStopDoesNotWaitForDescendantStderr(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	opts := options("stderr-child")
	opts.Args = append(opts.Args, pidFile)
	var process *Process
	t.Cleanup(func() {
		data, _ := os.ReadFile(pidFile)
		pid, _ := strconv.Atoi(string(data))
		if pid > 0 {
			if child, err := os.FindProcess(pid); err == nil {
				_ = child.Kill()
			}
		}
		if process != nil {
			process.Stop()
		}
	})
	var err error
	process, err = Start(context.Background(), session(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { process.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("UI reaping waited for descendant stderr")
	}
}

func TestResetReturnsFromAttachedAndDetachedGuests(t *testing.T) {
	for _, panel := range []bool{false, true} {
		s := session(t)
		opts := options("reset")
		opts.Panel = panel
		called := make(chan string, 1)
		opts.WindowAction = func(action string) bool { called <- action; return true }
		p, err := Start(context.Background(), s, opts)
		if err != nil {
			t.Fatal(err)
		}
		p.Wait()
		select {
		case action := <-called:
			if action != "reset" {
				t.Fatal("wrong navigation action")
			}
		default:
			t.Fatal("reset was not delivered")
		}
		if s.Active() {
			t.Fatal("reset retained the old guest session")
		}
	}
}
