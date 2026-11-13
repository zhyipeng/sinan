//go:build !windows

package transport

import "os/exec"

func configureProcess(*exec.Cmd) {}
