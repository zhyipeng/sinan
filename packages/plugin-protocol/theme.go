package protocol

// ThemeVersion describes data-only palettes. Future theme plugins can contribute
// definitions through a host provider; no CSS, URLs or scripts are part of it.
const ThemeVersion = "1"

type ThemeDefinition struct {
	Version    string            `json:"version"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Appearance string            `json:"appearance"`
	Tokens     map[string]string `json:"tokens"`
}
type ThemeState struct {
	Mode     string          `json:"mode"`
	Revision uint64          `json:"revision"`
	Light    ThemeDefinition `json:"light"`
	Dark     ThemeDefinition `json:"dark"`
	Error    string          `json:"error,omitempty"`
}
