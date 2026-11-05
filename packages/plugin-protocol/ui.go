package protocol

// UIBridgeVersion versions the narrow page bridge, independently of backend RPC.
const UIBridgeVersion = "1"

// UIView declares resources only. It does not grant host capabilities or make
// an HTML command available in the trusted Shell. Permissions default to none;
// this first bridge version rejects every nonempty permission request.
type UIView struct {
	ID            string   `json:"id"`
	Entry         string   `json:"entry"`
	BridgeVersion string   `json:"bridgeVersion"`
	Permissions   []string `json:"permissions,omitempty"`
}
