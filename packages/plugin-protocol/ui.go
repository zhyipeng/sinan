package protocol

import "encoding/json"

// UIBridgeVersion versions the narrow page bridge, independently of backend RPC.
const UIBridgeVersion = "1"
const UIRequestBridgeVersion = "2"

func SupportedUIBridge(v string) bool { return v == UIBridgeVersion || v == UIRequestBridgeVersion }

// UIView declares a fixed resource entry. Bridge 1 accepts no capabilities;
// bridge 2 explicitly allows same-plugin backend commands and private storage.
type UIView struct {
	Commands      []string `json:"commands,omitempty"`
	Title         string   `json:"title,omitempty"`
	ID            string   `json:"id"`
	Entry         string   `json:"entry"`
	BridgeVersion string   `json:"bridgeVersion"`
	Permissions   []string `json:"permissions,omitempty"`
}

// UIRequest has no caller identity or path; authority comes from the host session.
type UIRequest struct {
	Method    string          `json:"method"`
	CommandID string          `json:"commandId,omitempty"`
	Input     string          `json:"input,omitempty"`
	Revision  uint64          `json:"revision,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

// UILaunchContext contains only input explicitly passed when this view opened.
// A nil Input means a normal open; an empty string is still an explicit input.
type UILaunchContext struct {
	CommandID string         `json:"commandId,omitempty"`
	Input     *string        `json:"input,omitempty"`
	Image     *UILaunchImage `json:"image,omitempty"`
}
