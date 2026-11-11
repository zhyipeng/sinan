package protocol

import _ "embed"

//go:embed protocol.go
var scaffoldSource string

// ScaffoldSource returns the standalone backend protocol definitions.
func ScaffoldSource() string { return scaffoldSource }
