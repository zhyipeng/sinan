// The update helper never loads a WebView or plugin. It runs from private cache.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sinan/internal/buildinfo"
	"sinan/internal/hostupdate"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	plan, err := hostupdate.ReadPlan(os.Args[1])
	if err != nil || plan.Previous != buildinfo.Version {
		os.Exit(2)
	}
	fmt.Print("READY\n")
	if err = hostupdate.WaitParent(plan.Parent); err == nil {
		launched := false
		err = hostupdate.ReplaceAndRestart(plan, func(executable, session string) bool {
			ok := hostupdate.Launch(executable, session)
			launched = launched || ok
			return ok
		})
		if err != nil {
			hostupdate.SaveResult(err.Error())
			if !launched && hostupdate.ValidatePackage(plan.Target, plan.Previous) == nil {
				hostupdate.Launch(filepath.Join(hostupdate.BinaryDir(plan.Target), executableName()), "")
			}
		} else {
			hostupdate.SaveResult("已更新至 " + plan.Version)
		}
	} else {
		hostupdate.SaveResult(err.Error())
	}
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "sinan.exe"
	}
	return "sinan"
}
