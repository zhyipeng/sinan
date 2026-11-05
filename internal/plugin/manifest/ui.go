package manifest

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
)

var uiPathPattern = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func ValidViewID(id string) bool { return len(id) <= 64 && commandPattern.MatchString(id) }

// ValidUIEntry matches resource snapshot path rules, relative to fixed ui/.
func ValidUIEntry(entry string) bool {
	return len(entry) <= 256 && fs.ValidPath(entry) && path.Clean(entry) == entry &&
		uiPathPattern.MatchString(entry) && strings.Count(entry, "/") <= 8 && path.Ext(entry) == ".html"
}
