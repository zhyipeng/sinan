package distribution

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sinan/internal/plugin/ghmirror"
	"testing"
)

type chunkReader struct{ io.Reader }

func (r chunkReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:min(len(p), 16)]) }

func TestDownloadProgressUsesActualBytesAndResetsOnRetry(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "single-attempt"
		if retry {
			name = "mirror-retry"
		}
		t.Run(name, func(t *testing.T) {
			data := archive(t, []string{"manifest.json"}, false)
			a := artifact(data)
			f := New()
			f.SetConfig(ghmirror.Config{Mode: "auto"})
			attempts := 0
			f.Client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
				attempts++
				body := data
				if retry && attempts == 1 {
					body = data[:len(data)/2]
				}
				// Missing Content-Length must still use the reviewed catalog size.
				return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(chunkReader{bytes.NewReader(body)}), Header: http.Header{}}, nil
			})
			var counts []int64
			_, cleanup, err := f.StageWithProgress(context.Background(), t.TempDir(), a, func(downloaded, total int64) {
				if total != a.Size || downloaded < 0 || downloaded > total {
					t.Fatalf("invalid progress %d/%d", downloaded, total)
				}
				counts = append(counts, downloaded)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if len(counts) < 3 || counts[0] != 0 || counts[1] <= 0 || counts[1] >= a.Size || counts[len(counts)-1] != a.Size {
				t.Fatal("missing streamed progress", counts)
			}
			resets := 0
			var previous int64
			for _, count := range counts {
				if count == 0 {
					resets++
				} else if count < previous {
					t.Fatal("progress regressed within attempt", counts)
				}
				previous = count
			}
			expected := 1
			if retry {
				expected = 2
			}
			if attempts != expected || resets != expected {
				t.Fatal("attempt reset mismatch", attempts, resets)
			}
		})
	}
}
