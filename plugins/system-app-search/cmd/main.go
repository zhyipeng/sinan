package main

import (
	"context"
	"os"
	protocol "sinan/packages/plugin-protocol"
	sdk "sinan/packages/plugin-sdk"
	"sinan/plugins/system-app-search/catalog"
)

func main() {
	var index *catalog.Catalog
	if sdk.ServeWithHandlers(os.Stdin, os.Stdout, "system.app-search", sdk.Handlers{
		Initialize: func(p protocol.InitializeParams) error {
			index = catalog.NewWithCache(append(catalog.DefaultRoots(), p.Settings["directories"]...), p.CacheDir)
			return nil
		},
		Search: func(ctx context.Context, p protocol.SearchParams) (protocol.SearchResult, error) {
			return index.Search(ctx, p)
		},
		Command: func(ctx context.Context, p protocol.CommandParams) (protocol.CommandResult, error) {
			return index.Execute(ctx, p)
		},
	}) != nil {
		os.Exit(1)
	}
}
