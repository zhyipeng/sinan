// A trusted native renderer executable. stdin/stdout are private session pipes.
package main

import (
	"encoding/json"
	"os"
	"runtime"
	"sinan/internal/plugin/uihost"
	"sinan/internal/plugin/uiprocess"
)

func init() { runtime.LockOSThread() }

func run() bool {
	client, err := uiprocess.Connect(os.Stdin, os.Stdout)
	if err != nil {
		return false
	}
	defer client.Close()
	init := client.Init()
	result, err := uihost.Run(uihost.Config{Entry: init.Entry, CSP: init.CSP, Title: "司南插件"}, client, nil)
	if err != nil {
		return false
	}
	data, err := json.Marshal(result)
	return err == nil && client.Finish(data) == nil
}

func main() {
	if !run() {
		os.Exit(1)
	}
}
