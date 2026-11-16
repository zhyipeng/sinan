package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"sinan/internal/plugin/uiprocess"
)

func main() {
	mode := os.Args[1]
	if mode == "stderr-leaf" {
		time.Sleep(time.Minute)
		return
	}
	if mode == "stderr-child" {
		executable, _ := os.Executable()
		child := exec.Command(executable, "stderr-leaf")
		child.Stderr = os.Stderr
		if child.Start() != nil {
			os.Exit(18)
		}
		if os.WriteFile(os.Args[2], []byte(strconv.Itoa(child.Process.Pid)), 0600) != nil {
			_ = child.Process.Kill()
			os.Exit(19)
		}
	}
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
	if mode == "unsolicited-focused" {
		_ = c.Focused(1)
		return
	}
	if mode == "hello-only" {
		time.Sleep(time.Minute)
		return
	}
	if _, err = c.Message([]byte(`{"kind":"ready","version":1}`)); err != nil {
		os.Exit(9)
	}
	if mode == "large-image" {
		resource, ok := c.Resource(os.Args[2])
		if !ok {
			os.Exit(20)
		}
		report, _ := json.Marshal(map[string]int{"bytes": len(resource.Data)})
		_ = c.Finish(report)
		return
	}
	if mode == "settings" {
		time.Sleep(100 * time.Millisecond)
		if c.WindowAction("settings") != nil {
			os.Exit(18)
		}
	}
	if mode == "reset" {
		time.Sleep(100 * time.Millisecond)
		_ = c.WindowAction("reset")
		_ = c.Finish(json.RawMessage(`{"reset":true}`))
		return
	}
	if mode == "back" {
		time.Sleep(100 * time.Millisecond)
		_ = c.WindowAction("back")
		_ = c.Finish(json.RawMessage(`{"back":true}`))
		return
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
	if mode == "wait-revoke" || mode == "delay-focus" || mode == "panel" || mode == "wait-shortcut" || mode == "stderr-child" || mode == "wait-theme" || mode == "settings" {
		done := make(chan struct{})
		c.OnRevoke(func() { close(done) })
		delayed := false
		for {
			select {
			case control := <-c.Controls():
				if mode == "delay-focus" && !delayed {
					delayed = true
					time.Sleep(3 * time.Second)
				}
				if mode == "panel" && control.Kind == "detach" {
					if c.WindowAction("detach") != nil {
						os.Exit(17)
					}
				}
				if c.Focused(control.ID) != nil {
					os.Exit(16)
				}
			case <-done:
				report := json.RawMessage(`{"revoked":true}`)
				if mode == "wait-theme" {
					report, _ = json.Marshal(c.ThemeState())
				}
				if mode == "wait-shortcut" {
					report, _ = json.Marshal(map[string]string{"shortcut": c.DetachShortcut()})
				}
				if c.Finish(report) != nil {
					os.Exit(11)
				}
				return
			}
		}
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
