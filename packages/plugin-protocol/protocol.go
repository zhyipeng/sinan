// Package protocol defines the language-neutral v1 plugin wire contract.
package protocol

import "encoding/json"

const APIVersion = "1"
const MaxMessageBytes = 1 << 20

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

type InitializeParams struct {
	APIVersion string `json:"apiVersion"`
	PluginID   string `json:"pluginId"`
}

type InitializeResult struct {
	APIVersion string `json:"apiVersion"`
	PluginID   string `json:"pluginId"`
}

type CommandParams struct {
	CommandID string `json:"commandId"`
	Input     string `json:"input"`
}

type CommandResult struct {
	Text string `json:"text"`
}
