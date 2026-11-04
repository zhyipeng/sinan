package main

import (
	"errors"
	"os"
	"time"

	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
)

func main() {
	_ = sdk.Serve(os.Stdin, os.Stdout, "example.native", func(params protocol.CommandParams) (protocol.CommandResult, error) {
		switch params.CommandID {
		case "echo":
			return protocol.CommandResult{Text: params.Input}, nil
		case "hang":
			time.Sleep(time.Minute)
		case "crash":
			os.Exit(42)
		case "invalid":
			_, _ = os.Stdout.WriteString("not json\n")
			select {}
		}
		return protocol.CommandResult{}, errors.New("unknown command")
	})
}
