//go:build darwin && cgo

package searchtext

import (
	"slices"
	"testing"
)

func TestMandarinAliasesIncludeSpacedCompactAndInitialForms(t *testing.T) {
	aliases := Aliases("设置", "插件管理")
	for _, want := range []string{"设置", "she zhi", "shezhi", "sz", "插件管理", "cha jian guan li", "chajianguanli", "cjgl"} {
		if !slices.Contains(aliases, want) {
			t.Fatalf("missing alias %q", want)
		}
	}
}
