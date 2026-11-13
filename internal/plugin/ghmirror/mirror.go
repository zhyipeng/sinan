// Package ghmirror resolves optional GitHub acceleration mirrors for the plugin
// store. It is shared by the catalog reader and the release-asset downloader so
// both apply the same preference and template rules.
package ghmirror

import (
	"net/url"
	"strings"
)

// Placeholder is the only template token. It stands for the full original URL,
// e.g. https://api.github.com/repos/owner/repo/git/trees/main?recursive=1.
const Placeholder = "{url}"

// Mirror is one acceleration service.
type Mirror struct {
	ID       string
	Name     string
	Template string
}

// Builtins is the ordered, best-effort built-in list. Only services verified to
// proxy api.github.com are included. Availability is not guaranteed; callers
// always fall back to the official endpoint.
var Builtins = []Mirror{
	{ID: "chenc", Name: "github.chenc.dev", Template: "https://github.chenc.dev/" + Placeholder},
	{ID: "ghproxy", Name: "gh-proxy.com", Template: "https://gh-proxy.com/" + Placeholder},
}

// Config is the persisted mirror preference.
//
// Mode values:
//   - "" or "auto": try official first, then built-in mirrors (default product behaviour)
//   - "official": official endpoint only
//   - a built-in ID (e.g. "chenc"): try that mirror first, then official, then the rest
//   - "custom": try Template first, then official, then built-in mirrors
type Config struct {
	Mode     string
	Template string
}

// FromSettings builds a Config from stored settings values. Empty values resolve
// to the default "auto" behaviour.
func FromSettings(values map[string][]string) Config {
	mode := strings.TrimSpace(first(values["github-mirror"]))
	if mode == "" {
		mode = "auto"
	}
	return Config{Mode: mode, Template: strings.TrimSpace(first(values["github-mirror-template"]))}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func lookup(id string) (Mirror, bool) {
	for _, m := range Builtins {
		if m.ID == id {
			return m, true
		}
	}
	return Mirror{}, false
}

// Resolve expands a template with the original URL. It rejects templates that are
// not HTTPS or that omit the {url} placeholder, so a bad setting cannot switch
// scheme or silently point the store somewhere unexpected.
func Resolve(template, rawURL string) (string, bool) {
	if template == "" || len(template) > 2048 || !strings.Contains(template, Placeholder) {
		return "", false
	}
	out := strings.ReplaceAll(template, Placeholder, rawURL)
	parsed, err := url.Parse(out)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	return out, true
}

// Candidates returns the ordered request URLs to try for rawURL, most preferred
// first. Duplicate URLs are collapsed. The official URL is always a candidate
// unless the configured custom/built-in template already produced it.
func (c Config) Candidates(rawURL string) []string {
	official := strings.TrimSpace(rawURL)
	if official == "" {
		return nil
	}
	var out []string
	add := func(u string) {
		if u == "" {
			return
		}
		for _, existing := range out {
			if existing == u {
				return
			}
		}
		out = append(out, u)
	}
	addTemplate := func(template string) {
		if resolved, ok := Resolve(template, official); ok {
			add(resolved)
		}
	}
	addBuiltins := func(skipID string) {
		for _, m := range Builtins {
			if m.ID == skipID {
				continue
			}
			addTemplate(m.Template)
		}
	}

	switch c.Mode {
	case "", "auto":
		add(official)
		addBuiltins("")
	case "official":
		add(official)
	case "custom":
		addTemplate(c.Template)
		add(official)
		addBuiltins("")
	default:
		if m, ok := lookup(c.Mode); ok {
			addTemplate(m.Template)
			add(official)
			addBuiltins(m.ID)
		} else {
			// Unknown mode degrades to the default behaviour instead of failing.
			add(official)
			addBuiltins("")
		}
	}
	return out
}
