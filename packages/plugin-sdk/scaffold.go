package sdk

import "embed"

//go:embed server.go ui/index.js ui/index.d.ts
var scaffoldSources embed.FS

// ScaffoldSources returns the minimal SDK files needed by a standalone Go plugin.
// Each invocation returns independent bytes; no host implementation is included.
func ScaffoldSources() map[string][]byte {
	out := make(map[string][]byte)
	for _, name := range []string{"server.go", "ui/index.js", "ui/index.d.ts"} {
		out[name], _ = scaffoldSources.ReadFile(name)
	}
	return out
}
