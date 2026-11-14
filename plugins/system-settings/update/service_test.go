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
	if _, err := s.DownloadAndStage(context.Background()); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatal("download digest mismatch was accepted")
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
