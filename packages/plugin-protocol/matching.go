package protocol

// InputMatcher declares local matching without activating a plugin process.
// Matchers on a command are ORed. Regex uses Go/RE2 syntax and substring matching.
type InputMatcher struct {
	Kind      string   `json:"kind"`
	Pattern   string   `json:"pattern,omitempty"`
	JSONTypes []string `json:"jsonTypes,omitempty"`
	MinLength int      `json:"minLength,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
}
