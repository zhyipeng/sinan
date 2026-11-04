//go:build !darwin && !linux && !windows

package storage

import (
	"errors"
	"os"
)

func lockRoot(*os.File) error { return errors.New("unsupported root locking platform") }
