package catalog

import "sinan/packages/plugin-sdk/searchtext"

func searchNames(names ...string) []string { return searchtext.Aliases(names...) }
