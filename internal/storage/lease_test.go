package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRootLeaseAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.lock")
	lease, err := AcquireRootLease(path)
	if err != nil {
		t.Fatal(err)
	}
	run := func(mode string) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRootLeaseChild$")
		cmd.Env = []string{"SINAN_TEST_LEASE=" + path, "SINAN_TEST_LEASE_MODE=" + mode}
		if err := cmd.Run(); err != nil {
			t.Fatal("cross-process lease failed", err)
		}
	}
	run("blocked")
	lease.Close()
	lease.Close()
	run("exit")
	lease, err = AcquireRootLease(path)
	if err != nil {
		t.Fatal("process exit did not release lease", err)
	}
	lease.Close()
}
func TestRootLeaseChild(t *testing.T) {
	path := os.Getenv("SINAN_TEST_LEASE")
	if path == "" {
		return
	}
	lease, err := AcquireRootLease(path)
	if os.Getenv("SINAN_TEST_LEASE_MODE") == "blocked" {
		if err != nil {
			os.Exit(0)
		}
		lease.Close()
		os.Exit(2)
	}
	if err != nil {
		os.Exit(3)
	}
	// Exit intentionally without Close: a crashed owner must not leave a stale lock.
	os.Exit(0)
}
