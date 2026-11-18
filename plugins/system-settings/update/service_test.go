package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"sinan/internal/buildinfo"
	"sinan/internal/hostupdate"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func serviceForTest(t *testing.T, tag, assetURL, checksum string) *Service {
	t.Helper()
	version := strings.TrimPrefix(tag, "v")
	name := hostupdate.AssetName(version)
	base := "https://github.com/" + buildinfo.Repository + "/releases/download/" + tag + "/"
	if assetURL == "" {
		assetURL = base + name
	}
	payload, _ := json.Marshal(map[string]any{"tag_name": tag, "assets": []map[string]any{{"name": name, "browser_download_url": assetURL, "size": 3}, {"name": "SHA256SUMS.txt", "browser_download_url": base + "SHA256SUMS.txt", "size": 128}}})
	s := New("", "0.1.0", nil)
	s.state.Supported = true
	s.state.Phase = "idle"
	s.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("updater must not send credentials")
		}
		data := string(payload)
		switch req.URL.Path {
		case "/" + buildinfo.Repository + "/releases/download/" + tag + "/SHA256SUMS.txt":
			data = checksum
		case "/" + buildinfo.Repository + "/releases/download/" + tag + "/" + name:
			data = "bad"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: http.Header{}}, nil
	})}
	return s
}
func TestChecksBindVersionPlatformOriginAndDigest(t *testing.T) {
	hash := sha256.Sum256([]byte("zip"))
	checksum := hex.EncodeToString(hash[:]) + "  " + hostupdate.AssetName("0.2.0") + "\n"
	for _, tc := range []struct {
		name, tag, asset, checksum string
		good                       bool
	}{
		{"valid", "v0.2.0", "", checksum, true},
		{"external origin", "v0.2.0", "https://example.invalid/update.zip", checksum, false},
		{"invalid version", "v../0.2.0", "", checksum, false},
		{"pre-release", "v0.2.0-rc.1", "", checksum, false},
		{"missing checksum", "v0.2.0", "", "", false},
		{"duplicate checksum", "v0.2.0", "", checksum + checksum, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := serviceForTest(t, tc.tag, tc.asset, tc.checksum)
			state, err := s.Check(context.Background())
			if tc.good && (err != nil || !state.Available || state.LatestVersion != "0.2.0") {
				t.Fatalf("valid candidate rejected: %+v %v", state, err)
			}
			if !tc.good && err == nil {
				t.Fatal("unsafe candidate accepted")
			}
		})
	}
	s := serviceForTest(t, "v0.2.0", "", checksum)
	if _, err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	changes := make(chan State, 8)
	s.changed = func() { changes <- s.State() }
	if err := s.StartInstall(context.Background(), unexpectedHandoff(t)); err != nil {
		t.Fatal(err)
	}
	if state := waitForPhase(t, changes, "error"); !strings.Contains(state.Message, "SHA-256") {
		t.Fatal("download digest mismatch was accepted")
	}
}

func unexpectedHandoff(t *testing.T) func(hostupdate.Plan) error {
	t.Helper()
	return func(hostupdate.Plan) error {
		t.Error("invalid or cancelled update reached the installation helper")
		return context.Canceled
	}
}

func waitForPhase(t *testing.T, changes <-chan State, phase string) State {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case state := <-changes:
			if state.Phase == phase {
				return state
			}
		case <-timer.C:
			t.Fatalf("did not reach update phase %s", phase)
		}
	}
}

func TestBackgroundInstallTracksProgressAndResetsOnRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	hash := sha256.Sum256([]byte("zip"))
	checksum := hex.EncodeToString(hash[:]) + "  " + hostupdate.AssetName("0.2.0") + "\n"
	s := serviceForTest(t, "v0.2.0", "", checksum)
	if _, err := s.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	s.target = t.TempDir()
	changes := make(chan State, 8)
	s.changed = func() { changes <- s.State() }
	responses := make(chan *pausedDownload, 1)
	s.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		body := &pausedDownload{ctx: req.Context(), reading: make(chan struct{}), resume: make(chan struct{})}
		responses <- body
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), ContentLength: -1}, nil
	})
	start := func(ctx context.Context) *pausedDownload {
		t.Helper()
		if err := s.StartInstall(ctx, unexpectedHandoff(t)); err != nil {
			t.Fatal(err)
		}
		if state := waitForPhase(t, changes, "downloading"); state.DownloadedBytes != 0 || state.TotalBytes != 3 {
			t.Fatalf("initial download progress = %+v", state)
		}
		select {
		case body := <-responses:
			select {
			case <-body.reading:
				return body
			case <-time.After(5 * time.Second):
				t.Fatal("download did not write the first chunk")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("background download did not request asset")
		}
		return nil
	}

	body := start(t.Context())
	if state := s.State(); state.Phase != "downloading" || state.DownloadedBytes != 1 || state.TotalBytes != 3 {
		t.Fatalf("a new state reader could not obtain the partial download: %+v", state)
	}
	if err := s.StartInstall(t.Context(), unexpectedHandoff(t)); err == nil {
		t.Fatal("duplicate install accepted")
	}
	if _, err := s.Check(t.Context()); err == nil {
		t.Fatal("check accepted while download was active")
	}

	// No state reads are needed for the task to finish downloading and proceed to staging.
	close(body.resume)
	if state := waitForPhase(t, changes, "preparing"); state.DownloadedBytes != 3 {
		t.Fatalf("download did not reach the published asset size: %+v", state)
	}
	if state := waitForPhase(t, changes, "error"); !strings.Contains(state.Message, "更新准备失败") {
		t.Fatalf("invalid archive did not fail staging: %+v", state)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	start(ctx)
	cancel()
	if state := waitForPhase(t, changes, "error"); state.Message != "更新已取消" {
		t.Fatalf("application cancellation was not observed: %+v", state)
	}
}

type pausedDownload struct {
	ctx     context.Context
	reading chan struct{}
	resume  chan struct{}
	started bool
}

func (r *pausedDownload) Read(p []byte) (int, error) {
	if !r.started {
		r.started = true
		return copy(p, "z"), nil
	}
	close(r.reading)
	select {
	case <-r.resume:
		return copy(p, "ip"), io.EOF
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}
func TestNoDowngradeAndNoDevelopmentNetworkChecks(t *testing.T) {
	s := serviceForTest(t, "v0.1.0", "", "")
	state, err := s.Check(context.Background())
	if err != nil || state.Available || state.Phase != "current" {
		t.Fatalf("same version: %+v %v", state, err)
	}
	s = New("", "dev", nil)
	if _, err := s.Check(context.Background()); err == nil {
		t.Fatal("development build checked network")
	}
}
func TestRedirectRejectsCredentialsHTTPAndForeignHosts(t *testing.T) {
	for _, raw := range []string{"http://github.com/file", "https://example.invalid/file", "https://user:pass@github.com/file"} {
		parsed, _ := url.Parse(raw)
		if redirect(&http.Request{URL: parsed}, nil) == nil {
			t.Fatal("unsafe redirect accepted")
		}
	}
	parsed, _ := url.Parse("https://release-assets.githubusercontent.com/file")
	if redirect(&http.Request{URL: parsed}, nil) != nil {
		t.Fatal("GitHub asset redirect rejected")
	}
}
