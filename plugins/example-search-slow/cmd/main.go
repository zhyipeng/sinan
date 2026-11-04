package main

import (
	"os"
	"sinan/packages/plugin-sdk/searchdemo"
	"time"
)

func main() {
	if searchdemo.Run("example.search-slow", "延迟回声", 600*time.Millisecond) != nil {
		os.Exit(1)
	}
}
