package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"sinan/internal/plugin/uiprocess"
)

func main() {
	mode := os.Args[1]
	if mode == "crash" {
		os.Exit(7)
	}
	if mode == "silent" {
		time.Sleep(time.Minute)
		return
	}
	if mode == "bad" || mode == "oversize" || mode == "duplicate" || mode == "version" {
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		switch mode {
		case "bad":
			fmt.Println(`{"id":1,"kind":"hello","version":1,"pluginId":"other"}`)
		case "version":
			fmt.Println(`{"id":1,"kind":"hello","version":2}`)
		case "duplicate":
			fmt.Println(`{"id":1,"id":1,"kind":"hello","version":1}`)
		case "oversize":
			fmt.Println(strings.Repeat("x", 2<<20))
		}
		time.Sleep(time.Minute)
		return
	}
	c, err := uiprocess.Connect(os.Stdin, os.Stdout)
	if err != nil {
		os.Exit(8)
	}
	defer c.Close()
	if mode == "hello-only" {
		time.Sleep(time.Minute)
		return
	}
	if _, err = c.Message([]byte(`{"kind":"ready","version":1}`)); err != nil {
		os.Exit(9)
	}
	if mode == "hang" {
		time.Sleep(time.Minute)
		return
	}
	if mode == "ready-crash" {
		os.Exit(10)
	}
	if mode == "request-budget" || mode == "resource-budget" {
		url := "sinan-plugin://other.plugin/missing.html"
		if mode == "resource-budget" {
			url = strings.Replace(c.Init().Entry, "index.html", "large.js", 1)
		}
		for range 260 {
			if !c.Active() {
				break
			}
			c.Resource(url)
		}
		time.Sleep(time.Minute)
		return
	}
	if mode == "wait-revoke" {
		done := make(chan struct{})
		c.OnRevoke(func() { close(done) })
		<-done
		if c.Finish(json.RawMessage(`{"revoked":true}`)) != nil {
			os.Exit(11)
		}
		return
	}
	resource, own := c.Resource(c.Init().Entry)
	_, foreign := c.Resource("sinan-plugin://other.plugin/index.html")
	_, invalid := c.Message([]byte(`{"kind":"execute"}`))
	if !own || string(resource.Data) != "fixture" || foreign || invalid == nil || os.Getenv("SINAN_TEST_PARENT_MARKER") != "" {
		os.Exit(12)
	}
	if _, err = c.Message([]byte(`{"kind":"close"}`)); err != nil {
		os.Exit(13)
	}
	if _, stale := c.Resource(c.Init().Entry); stale {
		os.Exit(14)
	}
	if c.Finish(json.RawMessage(`{"ok":true}`)) != nil {
		os.Exit(15)
	}
}
