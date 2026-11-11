// Package theme validates and resolves data-only palettes. Plugin discovery and
// theme selection beyond the two builtins are deliberately not wired yet.
package theme

import (
	"embed"
	"encoding/json"
	"errors"
	"regexp"
	protocol "sinan/packages/plugin-protocol"
	"slices"
	"strings"
	"sync"
)

//go:embed builtin.json
var assets embed.FS
var tokens = []string{"background", "surface", "text", "muted", "placeholder", "line", "accent", "accentHover", "onAccent", "hover", "selected", "error", "errorBackground", "danger", "warning", "warningBackground", "warningLine", "outputLine"}
var colour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$`)

func ValidMode(mode string) bool { return mode == "system" || mode == "light" || mode == "dark" }
func Validate(def protocol.ThemeDefinition) error {
	if def.Version != protocol.ThemeVersion || len(def.ID) > 128 || !identifier.MatchString(def.ID) || strings.TrimSpace(def.Name) == "" || len(def.Name) > 128 || !slices.Contains([]string{"light", "dark"}, def.Appearance) || len(def.Tokens) != len(tokens) {
		return errors.New("主题定义无效")
	}
	for _, key := range tokens {
		if !colour.MatchString(def.Tokens[key]) {
			return errors.New("主题颜色必须是完整的十六进制颜色")
		}
	}
	return nil
}
func ValidateState(state protocol.ThemeState) error {
	if !ValidMode(state.Mode) || state.Light.Appearance != "light" || state.Dark.Appearance != "dark" || Validate(state.Light) != nil || Validate(state.Dark) != nil {
		return errors.New("主题状态无效")
	}
	return nil
}
func clone(def protocol.ThemeDefinition) protocol.ThemeDefinition {
	result := def
	result.Tokens = map[string]string{}
	for key, value := range def.Tokens {
		result.Tokens[key] = value
	}
	return result
}

// Registry is the future data-provider seam. Entries are namespaced and validated
// as a batch before publication. Registering does not execute plugin code.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]protocol.ThemeDefinition
}

func NewRegistry() *Registry { return &Registry{definitions: map[string]protocol.ThemeDefinition{}} }
func (r *Registry) Register(provider string, definitions []protocol.ThemeDefinition) error {
	if len(provider) > 128 || !identifier.MatchString(provider) || len(definitions) == 0 || len(definitions) > 16 {
		return errors.New("主题提供者无效")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, def := range definitions {
		if Validate(def) != nil || !strings.HasPrefix(def.ID, provider+".") {
			return errors.New("主题不属于提供者命名空间")
		}
		if _, exists := r.definitions[def.ID]; exists {
			return errors.New("主题 ID 已存在")
		}
	}
	seen := map[string]bool{}
	for _, def := range definitions {
		if seen[def.ID] {
			return errors.New("主题 ID 重复")
		}
		seen[def.ID] = true
	}
	for _, def := range definitions {
		r.definitions[def.ID] = clone(def)
	}
	return nil
}
func (r *Registry) Get(id string) (protocol.ThemeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[id]
	return clone(def), ok
}
func Builtins(mode string, revision uint64) protocol.ThemeState {
	data, _ := assets.ReadFile("builtin.json")
	var defs []protocol.ThemeDefinition
	if json.Unmarshal(data, &defs) != nil || len(defs) != 2 {
		panic("invalid embedded themes")
	}
	r := NewRegistry()
	if r.Register("system.theme", defs) != nil {
		panic("invalid embedded theme definitions")
	}
	light, _ := r.Get("system.theme.light")
	dark, _ := r.Get("system.theme.dark")
	return protocol.ThemeState{Mode: mode, Revision: revision, Light: light, Dark: dark}
}

func CloneState(state protocol.ThemeState) protocol.ThemeState {
	state.Light = clone(state.Light)
	state.Dark = clone(state.Dark)
	return state
}
