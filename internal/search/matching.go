package search

import (
	"strings"
	"sync"
	"unicode"

	"sinan/packages/plugin-sdk/searchtext"
)

// Command text is derived from each current snapshot; changed metadata gets new
// aliases. Bound retention so removed/updated plugin strings cannot accumulate.
var aliasCache = struct {
	sync.Mutex
	values map[string][]string
}{values: make(map[string][]string)}

func commandTextMatches(fields []string, query string) bool {
	query = strings.ToLower(query)
	if strings.Contains(strings.ToLower(strings.Join(fields, " ")), query) {
		return true
	}
	for _, field := range fields {
		if !strings.ContainsFunc(field, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			continue
		}
		aliasCache.Lock()
		aliases, ok := aliasCache.values[field]
		aliasCache.Unlock()
		if !ok {
			aliases = searchtext.Aliases(field)
			// Oversized text still matches, but is never retained in the cache.
			if len(field) <= 2048 {
				aliasCache.Lock()
				if len(aliasCache.values) >= 2048 {
					clear(aliasCache.values)
				}
				aliasCache.values[field] = aliases
				aliasCache.Unlock()
			}
		}
		for _, alias := range aliases {
			if strings.Contains(alias, query) {
				return true
			}
		}
	}
	return false
}
