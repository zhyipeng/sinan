// Package searchtext provides shared text aliases without owning search providers or indexes.
package searchtext

import (
	"strings"
	"unicode"
)

// Aliases returns normalized originals plus full pinyin and initials where supported.
func Aliases(names ...string) []string {
	result := []string{}
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		add(name)
		if !strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			continue
		}
		latin := transliterate(name)
		if latin == "" {
			continue
		}
		add(latin)
		words := strings.Fields(latin)
		add(strings.Join(words, ""))
		initials := ""
		for _, word := range words {
			for _, r := range word {
				initials += string(r)
				break
			}
		}
		add(initials)
	}
	return result
}
