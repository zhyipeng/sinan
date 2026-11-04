package main

import (
	"os"
	sdk "sinan/packages/plugin-sdk"
	"sinan/plugins/system-app-search/catalog"
)

func main() {
	catalog := catalog.New(catalog.DefaultRoots())
	if sdk.ServeWithHandlers(os.Stdin, os.Stdout, "system.app-search", sdk.Handlers{Search: catalog.Search, Command: catalog.Execute}) != nil {
		os.Exit(1)
	}
}
