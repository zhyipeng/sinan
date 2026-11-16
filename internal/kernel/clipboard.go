package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sinan/internal/plugin/transport"
	protocol "sinan/packages/plugin-protocol"
	"slices"
)

// Capture authority from the installed manifest, never from request parameters.
func clipboardHandler(permissions []string, call func(context.Context, string, protocol.ClipboardParams) (protocol.ClipboardResult, error)) transport.HostHandler {
	permissions = append([]string(nil), permissions...)
	return func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		if (method != "clipboard.read" && method != "clipboard.write") || !slices.Contains(permissions, method) {
			return nil, errors.New("未声明宿主能力权限")
		}
		var params protocol.ClipboardParams
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&params) != nil {
			return nil, errors.New("剪贴板参数无效")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return nil, errors.New("剪贴板参数无效")
		}
		return call(ctx, method, params)
	}
}
