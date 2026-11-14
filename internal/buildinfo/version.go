// Package buildinfo contains host identity injected by the native build entry.
package buildinfo

import (
	"regexp"
	"strings"
)

var Version = "dev"

const Repository = "zhyipeng/sinan"

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func ValidVersion(version string) bool {
	if len(version) > 128 {
		return false
	}
	parts := versionPattern.FindStringSubmatch(version)
	if parts == nil {
		return false
	}
	for _, part := range strings.Split(parts[4], ".") {
		if numeric(part) && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}
func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func compareNumber(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

// Compare requires validated versions and implements SemVer precedence.
func Compare(a, b string) int {
	aa, bb := versionPattern.FindStringSubmatch(a), versionPattern.FindStringSubmatch(b)
	if aa == nil || bb == nil {
		panic("compare requires valid versions")
	}
	for i := 1; i <= 3; i++ {
		if c := compareNumber(aa[i], bb[i]); c != 0 {
			return c
		}
	}
	if aa[4] == bb[4] {
		return 0
	}
	if aa[4] == "" {
		return 1
	}
	if bb[4] == "" {
		return -1
	}
	ap, bp := strings.Split(aa[4], "."), strings.Split(bb[4], ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		an, bn := numeric(ap[i]), numeric(bp[i])
		if an && bn {
			return compareNumber(ap[i], bp[i])
		}
		if an {
			return -1
		}
		if bn {
			return 1
		}
		return strings.Compare(ap[i], bp[i])
	}
	if len(ap) < len(bp) {
		return -1
	}
	return 1
}
