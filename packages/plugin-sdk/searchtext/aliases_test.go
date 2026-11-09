package searchtext

import (
	"slices"
	"testing"
)

func TestAliasesKeepNormalizedOriginalsAndRemoveDuplicates(t *testing.T) {
	aliases := Aliases(" Tool ", "tool", "", "工具", "工具")
	if len(aliases) < 2 || aliases[0] != "tool" || !slices.Contains(aliases, "工具") {
		t.Fatal("original text lost")
	}
	seen := map[string]bool{}
	for _, alias := range aliases {
		if alias == "" || seen[alias] {
			t.Fatal("empty or duplicate alias")
		}
		seen[alias] = true
	}
}
