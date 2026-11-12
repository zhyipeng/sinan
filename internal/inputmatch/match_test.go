package inputmatch

import (
	"fmt"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

func TestJSONTypesRequireValidWholeDocument(t *testing.T) {
	rules := []protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object", "array"}}}
	for _, input := range []string{`{}`, `[]`, " \n {\"large\":900719925474099312345,\"nested\":[true,null]} \t", `[{},[]]`} {
		if !NewQuery(input).MatchAny(rules) {
			t.Fatal("valid container did not match")
		}
	}
	for _, input := range []string{``, `null`, `123`, `true`, `"text"`, `{bad}`, `{} trailing`, `[] {}`, `{"a":NaN}`, "\u00a0{}", string([]byte{'[', '"', 0xff, '"', ']'})} {
		if NewQuery(input).MatchAny(rules) {
			t.Fatal("invalid container matched")
		}
	}
	for kind, input := range map[string]string{"object": "{}", "array": "[]", "string": `"x"`, "number": "1e100", "boolean": "false", "null": "null"} {
		if !NewQuery(input).MatchAny([]protocol.InputMatcher{{Kind: "json", JSONTypes: []string{kind}}}) {
			t.Fatal("generic JSON type did not match")
		}
	}
}
func TestRegexSyntaxLengthAndOR(t *testing.T) {
	rules := []protocol.InputMatcher{{Kind: "regex", Pattern: `^https?://[^\s]+$`, MinLength: 8, MaxLength: 100}, {Kind: "regex", Pattern: `(?i)^mailto:`}}
	for _, input := range []string{"https://example.test", "MAILTO:a@example.test"} {
		if !NewQuery(input).MatchAny(rules) {
			t.Fatal("valid regex did not match")
		}
	}
	for _, input := range []string{"no url", " https://example.test", "https://" + strings.Repeat("x", 100)} {
		if NewQuery(input).MatchAny(rules) {
			t.Fatal("invalid or oversized regex matched")
		}
	}
	if NewQuery("中").MatchAny([]protocol.InputMatcher{{Kind: "regex", Pattern: ".", MaxLength: 2}}) {
		t.Fatal("length must count UTF-8 bytes")
	}
}
func TestRejectInvalidOrExpensiveRules(t *testing.T) {
	for _, rule := range []protocol.InputMatcher{
		{Kind: "unknown"}, {Kind: "regex"}, {Kind: "regex", Pattern: "("}, {Kind: "regex", Pattern: `(a)\1`}, {Kind: "regex", Pattern: `(?=a)`},
		{Kind: "regex", Pattern: strings.Repeat("a", 257)}, {Kind: "regex", Pattern: `a{300}`},
		{Kind: "regex", Pattern: "x", JSONTypes: []string{"object"}}, {Kind: "regex", Pattern: "x", MinLength: 5, MaxLength: 2},
		{Kind: "json"}, {Kind: "json", JSONTypes: []string{"object", "object"}}, {Kind: "json", JSONTypes: []string{"any"}},
		{Kind: "json", JSONTypes: []string{"object"}, Pattern: "x"}, {Kind: "json", JSONTypes: []string{"array"}, MaxLength: MaxInputBytes + 1},
	} {
		if Validate(rule) == nil {
			t.Fatal("invalid rule accepted")
		}
	}
	for _, pattern := range []string{`^(a+)+$`, `(?i)^hello`, `^.{1,100}$`} {
		if err := Validate(protocol.InputMatcher{Kind: "regex", Pattern: pattern}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestQueryBudgetsAndOversizedInput(t *testing.T) {
	rule := []protocol.InputMatcher{{Kind: "regex", Pattern: `^a+$`}}
	q := NewQuery(strings.Repeat("a", MaxInputBytes))
	matched := 0
	for range MaxQueryMatchers {
		if q.MatchAny(rule) {
			matched++
		}
	}
	if matched == 0 || matched == MaxQueryMatchers || q.remaining < 0 {
		t.Fatal("regex work budget not enforced")
	}
	q = NewQuery("x")
	for range MaxQueryMatchers {
		q.MatchAny([]protocol.InputMatcher{{Kind: "regex", Pattern: "nomatch"}})
	}
	if q.MatchAny([]protocol.InputMatcher{{Kind: "regex", Pattern: "x"}}) {
		t.Fatal("rule count budget not enforced")
	}
	if NewQuery(strings.Repeat("a", MaxInputBytes+1)).MatchAny(rule) {
		t.Fatal("oversized input matched")
	}
	q = NewQuery("[]")
	q.remaining = 0
	if !q.MatchAny([]protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"array"}}}) {
		t.Fatal("regex exhaustion blocked JSON validation")
	}
}
func TestCompiledCacheIsBounded(t *testing.T) {
	for i := 0; i < maxCachedPatterns+10; i++ {
		if _, err := compile(fmt.Sprintf("^test-%d$", i)); err != nil {
			t.Fatal(err)
		}
	}
	patterns.Lock()
	defer patterns.Unlock()
	if len(patterns.values) > maxCachedPatterns || len(patterns.order) > maxCachedPatterns {
		t.Fatal("unbounded regex cache")
	}
}
func BenchmarkRegexNestedQuantifiers64KiB(b *testing.B) {
	input := strings.Repeat("a", MaxInputBytes-1) + "!"
	rules := []protocol.InputMatcher{{Kind: "regex", Pattern: `^(a+)+$`}}
	for b.Loop() {
		NewQuery(input).MatchAny(rules)
	}
}
func BenchmarkJSONContainer64KiB(b *testing.B) {
	input := `{"text":"` + strings.Repeat("x", MaxInputBytes-11) + `"}`
	rules := []protocol.InputMatcher{{Kind: "json", JSONTypes: []string{"object", "array"}}}
	for b.Loop() {
		NewQuery(input).MatchAny(rules)
	}
}

func TestImageRuleDoesNotMatchText(t *testing.T) {
	if err := Validate(protocol.InputMatcher{Kind: "image"}); err != nil {
		t.Fatal(err)
	}
	for _, rule := range []protocol.InputMatcher{{Kind: "image", Pattern: "x"}, {Kind: "image", JSONTypes: []string{"object"}}, {Kind: "image", MaxLength: 1}} {
		if Validate(rule) == nil {
			t.Fatal("image accepted text options")
		}
	}
	if NewQuery("image.png").MatchAny([]protocol.InputMatcher{{Kind: "image"}}) {
		t.Fatal("filename matched image content")
	}
}
