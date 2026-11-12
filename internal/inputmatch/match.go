// Package inputmatch implements bounded declarative input matching. It executes
// no plugin code and never retains input text in caches.
package inputmatch

import (
	"encoding/json"
	"errors"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
	"unicode/utf8"

	protocol "sinan/packages/plugin-protocol"
)

const MaxInputBytes = 64 << 10
const MaxMatchersPerCommand = 8
const MaxMatchersPerPlugin = 32
const MaxPatternBytes = 256
const MaxRegexInstructions = 256
const MaxRegexWork = 8 << 20
const MaxQueryMatchers = 256
const maxCachedPatterns = 256

var patterns = struct {
	sync.Mutex
	values map[string]compiledPattern
	order  []string
}{values: map[string]compiledPattern{}}

type compiledPattern struct {
	re   *regexp.Regexp
	cost int
}

func compile(pattern string) (compiledPattern, error) {
	if len(pattern) == 0 || len(pattern) > MaxPatternBytes {
		return compiledPattern{}, errors.New("正则表达式长度无效")
	}
	patterns.Lock()
	defer patterns.Unlock()
	if re, ok := patterns.values[pattern]; ok {
		return re, nil
	}
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return compiledPattern{}, errors.New("正则表达式无效；仅支持 Go/RE2 语法")
	}
	program, err := syntax.Compile(parsed.Simplify())
	if err != nil || len(program.Inst) > MaxRegexInstructions {
		return compiledPattern{}, errors.New("正则表达式复杂度超过限制")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return compiledPattern{}, errors.New("正则表达式无效")
	}
	if len(patterns.order) >= maxCachedPatterns {
		delete(patterns.values, patterns.order[0])
		patterns.order = patterns.order[1:]
	}
	compiled := compiledPattern{re: re, cost: len(program.Inst)}
	patterns.values[pattern] = compiled
	patterns.order = append(patterns.order, pattern)
	return compiled, nil
}

func Validate(rule protocol.InputMatcher) error {
	if rule.MinLength < 0 || rule.MaxLength < 0 || rule.MinLength > MaxInputBytes || rule.MaxLength > MaxInputBytes || rule.MaxLength > 0 && rule.MinLength > rule.MaxLength {
		return errors.New("输入匹配长度限制无效（单位为 UTF-8 字节）")
	}
	switch rule.Kind {
	case "regex":
		if len(rule.JSONTypes) > 0 {
			return errors.New("正则规则不能声明 JSON 类型")
		}
		_, err := compile(rule.Pattern)
		return err
	case "image":
		if rule.Pattern != "" || len(rule.JSONTypes) > 0 || rule.MinLength != 0 || rule.MaxLength != 0 {
			return errors.New("图片匹配不支持文本规则字段")
		}
		return nil
	case "json":
		if rule.Pattern != "" || len(rule.JSONTypes) == 0 || len(rule.JSONTypes) > 6 {
			return errors.New("JSON 匹配规则无效")
		}
		seen := map[string]bool{}
		for _, kind := range rule.JSONTypes {
			switch kind {
			case "object", "array", "string", "number", "boolean", "null":
			default:
				return errors.New("JSON 顶层类型无效")
			}
			if seen[kind] {
				return errors.New("JSON 顶层类型重复")
			}
			seen[kind] = true
		}
		return nil
	default:
		return errors.New("不支持的输入匹配规则")
	}
}

// Query is private to one search. JSON validation is performed at most once;
// regex work and rule evaluations are bounded across all installed plugins.
type Query struct {
	text      string
	valid     bool
	checked   bool
	jsonKind  string
	remaining int
	rules     int
}

func NewQuery(text string) *Query {
	return &Query{text: text, valid: len(text) <= MaxInputBytes && utf8.ValidString(text), remaining: MaxRegexWork, rules: MaxQueryMatchers}
}
func (q *Query) MatchAny(rules []protocol.InputMatcher) bool {
	if len(rules) == 0 || !q.valid {
		return false
	}
	for _, rule := range rules {
		if q.rules <= 0 {
			return false
		}
		q.rules--
		if len(q.text) < rule.MinLength || rule.MaxLength > 0 && len(q.text) > rule.MaxLength {
			continue
		}
		switch rule.Kind {
		case "regex":
			re, err := compile(rule.Pattern)
			if err != nil {
				continue
			}
			// Charge input bytes × compiled RE2 instructions across all plugins.
			work := len(q.text) * re.cost
			if work > q.remaining {
				continue
			}
			q.remaining -= work
			if re.re.MatchString(q.text) {
				return true
			}
		case "json":
			if !q.checked {
				q.checked = true
				trimmed := strings.TrimSpace(q.text)
				if len(trimmed) > 0 && json.Valid([]byte(q.text)) {
					switch trimmed[0] {
					case '{':
						q.jsonKind = "object"
					case '[':
						q.jsonKind = "array"
					case '"':
						q.jsonKind = "string"
					case 't', 'f':
						q.jsonKind = "boolean"
					case 'n':
						q.jsonKind = "null"
					default:
						q.jsonKind = "number"
					}
				}
			}
			for _, kind := range rule.JSONTypes {
				if q.jsonKind != "" && kind == q.jsonKind {
					return true
				}
			}
		}
	}
	return false
}
