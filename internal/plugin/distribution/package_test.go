package distribution

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func archive(t *testing.T, names []string, symlink bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0755)
		if symlink {
			header.SetMode(os.ModeSymlink | 0777)
		}
		file, err := z.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func fixtureFetcher(data []byte) *Fetcher {
	fetcher := New()
	fetcher.Client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})
	return fetcher
}
func artifact(data []byte) Artifact {
	sum := sha256.Sum256(data)
	return Artifact{Repository: "example/plugin", ID: 7, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
}
func TestBoundedStagePreservesExecutableAndCleansUp(t *testing.T) {
	data := archive(t, []string{"manifest.json", "backend/plugin"}, false)
	cache := t.TempDir()
	dir, cleanup, err := fixtureFetcher(data).Stage(context.Background(), cache, artifact(data))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "backend/plugin"))
	if err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatal("executable mode lost")
	}
	cleanup()
	entries, _ := os.ReadDir(cache)
	if len(entries) != 0 {
		t.Fatal("staging cleanup failed")
	}
}
func TestUnsafeArchivesNeverLeaveStagedFiles(t *testing.T) {
	for _, test := range []struct {
		names []string
		link  bool
	}{{[]string{"../escape"}, false}, {[]string{"/absolute"}, false}, {[]string{"dir\\escape"}, false}, {[]string{"manifest.json", "MANIFEST.JSON"}, false}, {[]string{"manifest.json", "link"}, true}, {[]string{"nested/manifest.json"}, false}} {
		data := archive(t, test.names, test.link)
		cache := t.TempDir()
		if _, _, err := fixtureFetcher(data).Stage(context.Background(), cache, artifact(data)); err == nil {
			t.Fatal("unsafe ZIP accepted")
		}
		entries, _ := os.ReadDir(cache)
		if len(entries) != 0 {
			t.Fatal("failed staging leaked files")
		}
	}
}
func TestDigestMismatchAndInflationAreRejected(t *testing.T) {
	data := archive(t, []string{"manifest.json"}, false)
	a := artifact(data)
	a.SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err := fixtureFetcher(data).Stage(context.Background(), t.TempDir(), a); err == nil {
		t.Fatal("bad digest accepted")
	}
	index := bytes.Index(data, []byte{'P', 'K', 1, 2})
	if index < 0 {
		t.Fatal("no ZIP central record")
	}
	binary.LittleEndian.PutUint32(data[index+24:index+28], uint32(64<<20)+1)
	if _, _, err := fixtureFetcher(data).Stage(context.Background(), t.TempDir(), artifact(data)); err == nil {
		t.Fatal("oversized inflation accepted")
	}
}
func TestPartialDownloadReportsTransferErrorAndCleansUp(t *testing.T) {
	data := archive(t, []string{"manifest.json"}, false)
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"timeout", context.DeadlineExceeded, "插件包下载超时，请重试"},
		{"interrupted", io.ErrUnexpectedEOF, "插件包下载中断"},
		{"wrong-size", io.EOF, "插件包大小或 SHA-256 校验失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := t.TempDir()
			f := New()
			f.Client.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
				body := io.MultiReader(bytes.NewReader(data[:len(data)/2]), iotest.ErrReader(tc.err))
				return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(body)}, nil
			})
			if _, _, err := f.Stage(context.Background(), cache, artifact(data)); err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			entries, err := os.ReadDir(cache)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed staging left cache entries: %v, %v", entries, err)
			}
		})
	}
}

func TestRedirectCannotEscapeApprovedGitHubHosts(t *testing.T) {
	f := New()
	calls := 0
	f.Client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.invalid/archive.zip"}}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	if _, _, err := f.Stage(context.Background(), t.TempDir(), artifact([]byte("fixture"))); err == nil || calls != 1 {
		t.Fatal("untrusted redirect followed")
	}
}

func TestLocalStageSnapshotAndCleanup(t *testing.T) {
	source := filepath.Join(t.TempDir(), "plugin.ZIP")
	if err := os.WriteFile(source, archive(t, []string{"manifest.json", "backend/plugin"}, false), 0600); err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := StageLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err = os.WriteFile(source, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || string(data) != "fixture" {
		t.Fatal("staged snapshot changed with source")
	}
	info, err := os.Stat(filepath.Join(dir, "backend/plugin"))
	if err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatal("executable mode lost")
	}
	cleanup()
	if _, err = os.Stat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Fatal("local staging not removed")
	}
}

func TestLocalStageRejectsUnsafeAndOversizeArchives(t *testing.T) {
	source := filepath.Join(t.TempDir(), "plugin.zip")
	for _, data := range [][]byte{[]byte("not a zip"), archive(t, []string{"manifest.json", "../escape"}, false), archive(t, []string{"nested/manifest.json"}, false), archive(t, []string{"manifest.json", "link"}, true)} {
		if err := os.WriteFile(source, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := StageLocal(source); err == nil {
			t.Fatal("unsafe local ZIP accepted")
		}
	}
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate((64 << 20) + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = StageLocal(source); err == nil {
		t.Fatal("oversized local archive accepted")
	}
}
