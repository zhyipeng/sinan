// Package searchdemo supplies the shared implementation of the two demo processes.
package searchdemo

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
)

func Run(id, name string, delay time.Duration) error {
	return sdk.ServeWithHandlers(os.Stdin, os.Stdout, id, sdk.Handlers{
		Command: func(ctx context.Context, p protocol.CommandParams) (protocol.CommandResult, error) {
			if p.CommandID != "echo" {
				return protocol.CommandResult{}, errors.New("unknown command")
			}
			return protocol.CommandResult{Text: p.Input}, nil
		},
		Search: func(ctx context.Context, p protocol.SearchParams) (protocol.SearchResult, error) {
			if p.ProviderID != "demo" {
				return protocol.SearchResult{}, errors.New("unknown provider")
			}
			// Prefixes make delay and failure reproducible without environment variables.
			wait := delay
			if strings.HasPrefix(p.Query, "slow:") {
				wait = time.Second
			}
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return protocol.SearchResult{}, ctx.Err()
			case <-timer.C:
			}
			if strings.Contains(p.Query, "fail") && delay > 0 {
				return protocol.SearchResult{}, errors.New("demo failure")
			}
			return protocol.SearchResult{Items: []protocol.SearchItem{{ID: "echo", Title: name + "：" + p.Query, Subtitle: "示例搜索结果", CommandID: "echo", Input: p.Query, Score: .7}}}, nil
		},
	})
}
