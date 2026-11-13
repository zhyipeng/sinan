package ghmirror

import (
	"reflect"
	"testing"
)

const raw = "https://api.github.com/repos/zhyipeng/sinan-plugin-registry/git/trees/main?recursive=1"

func TestResolve(t *testing.T) {
	got, ok := Resolve("https://github.chenc.dev/{url}", raw)
	if !ok || got != "https://github.chenc.dev/"+raw {
		t.Fatalf("resolve chenc: %q %v", got, ok)
	}
	for _, bad := range []string{"", "https://x.example/", "http://x.example/{url}", "ftp://x/{url}", "https:///x/{url}"} {
		if _, ok := Resolve(bad, raw); ok {
			t.Fatalf("accepted invalid template %q", bad)
		}
	}
}

func TestCandidatesOrder(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "default auto official first",
			cfg:  Config{Mode: "auto"},
			want: []string{raw, "https://github.chenc.dev/" + raw, "https://gh-proxy.com/" + raw},
		},
		{
			name: "official only",
			cfg:  Config{Mode: "official"},
			want: []string{raw},
		},
		{
			name: "named mirror first",
			cfg:  Config{Mode: "chenc"},
			want: []string{"https://github.chenc.dev/" + raw, raw, "https://gh-proxy.com/" + raw},
		},
		{
			name: "custom first",
			cfg:  Config{Mode: "custom", Template: "https://mirror.example/{url}"},
			want: []string{"https://mirror.example/" + raw, raw, "https://github.chenc.dev/" + raw, "https://gh-proxy.com/" + raw},
		},
		{
			name: "unknown mode degrades to auto",
			cfg:  Config{Mode: "does-not-exist"},
			want: []string{raw, "https://github.chenc.dev/" + raw, "https://gh-proxy.com/" + raw},
		},
		{
			name: "invalid custom template skipped",
			cfg:  Config{Mode: "custom", Template: "not-a-url"},
			want: []string{raw, "https://github.chenc.dev/" + raw, "https://gh-proxy.com/" + raw},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Candidates(raw); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("candidates = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFromSettings(t *testing.T) {
	if got := FromSettings(nil); got.Mode != "auto" {
		t.Fatalf("empty values = %#v", got)
	}
	got := FromSettings(map[string][]string{"github-mirror": {"custom"}, "github-mirror-template": {" https://m.example/{url} "}})
	if got.Mode != "custom" || got.Template != "https://m.example/{url}" {
		t.Fatalf("from settings = %#v", got)
	}
}
