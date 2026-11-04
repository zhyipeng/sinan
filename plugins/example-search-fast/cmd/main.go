package main

import (
	"os"
	"sinan/packages/plugin-sdk/searchdemo"
)

func main() {
	if searchdemo.Run("example.search-fast", "即时回声", 0) != nil {
		os.Exit(1)
	}
}
