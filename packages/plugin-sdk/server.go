// Package sdk is the minimal Go SDK for trusted native plugins.
package sdk

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"

	protocol "sinan/packages/plugin-protocol"
)

type CommandHandler func(protocol.CommandParams) (protocol.CommandResult, error)

func Serve(in io.Reader, out io.Writer, pluginID string, handler CommandHandler) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), protocol.MaxMessageBytes)
	encoder := json.NewEncoder(out)
	initialized := false
	for scanner.Scan() {
		var req protocol.Request
		if json.Unmarshal(scanner.Bytes(), &req) != nil || req.JSONRPC != "2.0" || req.ID == 0 {
			return errors.New("invalid request")
		}
		res := protocol.Response{JSONRPC: "2.0", ID: req.ID}
		var result any
		switch req.Method {
		case "initialize":
			var params protocol.InitializeParams
			if json.Unmarshal(req.Params, &params) != nil || params.APIVersion != protocol.APIVersion || params.PluginID != pluginID {
				res.Error = &protocol.Error{Code: -32602, Message: "incompatible plugin"}
			} else {
				initialized = true
				result = protocol.InitializeResult{APIVersion: protocol.APIVersion, PluginID: pluginID}
			}
		case "command.execute":
			var params protocol.CommandParams
			if !initialized {
				res.Error = &protocol.Error{Code: -32002, Message: "not initialized"}
			} else if json.Unmarshal(req.Params, &params) != nil {
				res.Error = &protocol.Error{Code: -32602, Message: "invalid params"}
			} else {
				value, err := handler(params)
				if err != nil {
					res.Error = &protocol.Error{Code: -32000, Message: "command failed"}
				} else {
					result = value
				}
			}
		default:
			res.Error = &protocol.Error{Code: -32601, Message: "method not found"}
		}
		if res.Error == nil {
			var err error
			res.Result, err = json.Marshal(result)
			if err != nil || len(res.Result) > protocol.MaxMessageBytes/2 {
				res.Result = nil
				res.Error = &protocol.Error{Code: -32000, Message: "invalid result"}
			}
		}
		if err := encoder.Encode(res); err != nil {
			return err
		}
	}
	return scanner.Err()
}
