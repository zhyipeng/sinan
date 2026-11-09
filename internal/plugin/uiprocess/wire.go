// Package uiprocess supervises host-selected UI executables over session-private
// stdio pipes. This is an internal protocol, not the backend plugin RPC API.
package uiprocess

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"sinan/internal/plugin/uiview"
)

const maxFrameBytes = 1 << 20
const maxRequests = 256
const maxResourceBytes = 8 << 20

type Init struct {
	BridgeVersion  string `json:"bridgeVersion,omitempty"`
	DetachShortcut string `json:"detachShortcut,omitempty"`
	Panel          bool   `json:"panel,omitempty"`
	Title          string `json:"title,omitempty"`
	Kind           string `json:"kind"`
	Version        int    `json:"version"`
	Entry          string `json:"entry"`
	CSP            string `json:"csp"`
	ProbeTarget    string `json:"probeTarget,omitempty"`
}
type request struct {
	WindowAction string          `json:"windowAction,omitempty"`
	Control      uint64          `json:"control,omitempty"`
	ID           uint64          `json:"id"`
	Kind         string          `json:"kind"`
	Version      int             `json:"version,omitempty"`
	URL          string          `json:"url,omitempty"`
	Message      json.RawMessage `json:"message,omitempty"`
	Report       json.RawMessage `json:"report,omitempty"`
}
type response struct {
	Result         json.RawMessage  `json:"result,omitempty"`
	Error          string           `json:"error,omitempty"`
	DetachShortcut string           `json:"detachShortcut,omitempty"`
	Kind           string           `json:"kind"`
	ID             uint64           `json:"id,omitempty"`
	OK             bool             `json:"ok,omitempty"`
	Action         uiview.Action    `json:"action,omitempty"`
	Resource       *uiview.Resource `json:"resource,omitempty"`
}

func newScanner(reader io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 4096), maxFrameBytes)
	return s
}
func decode(data []byte, out any) error {
	// Reject duplicate envelope keys before decoding the typed role-specific schema.
	// encoding/json otherwise accepts case-insensitive aliases of struct fields.
	allowed := map[string]bool{}
	t := reflect.TypeOf(out).Elem()
	for i := 0; i < t.NumField(); i++ {
		allowed[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	keys := json.NewDecoder(bytes.NewReader(data))
	token, err := keys.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("invalid envelope")
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err = keys.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok || !allowed[name] || seen[name] {
			return errors.New("duplicate envelope field")
		}
		seen[name] = true
		var value json.RawMessage
		if err = keys.Decode(&value); err != nil {
			return err
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing envelope content")
	}
	return nil
}
func writeFrame(writer io.Writer, value any) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil || buffer.Len() >= maxFrameBytes {
		return errors.New("UI 管道消息超过限制")
	}
	_, err := writer.Write(buffer.Bytes())
	return err
}
