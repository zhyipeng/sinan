package main

import (
	"errors"
	"os"
	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
)

func main() {
	err := sdk.Serve(os.Stdin, os.Stdout, "example.echo", func(params protocol.CommandParams) (protocol.CommandResult, error) {
		if params.CommandID != "echo" {
			return protocol.CommandResult{}, errors.New("unknown command")
		}
		text := params.Input
		if text == "" {
			text = "来自独立进程的问候。"
		}
		return protocol.CommandResult{Text: text}, nil
	})
	if err != nil {
		os.Exit(1)
	}
}
