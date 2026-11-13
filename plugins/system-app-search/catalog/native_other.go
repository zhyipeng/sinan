//go:build (!darwin && !windows && !linux) || (darwin && !cgo)

package catalog

import (
	"context"
	"errors"
)

func readMetadata(string) (metadata, error) {
	return metadata{}, errors.New("application search requires macOS with cgo")
}

func readIcon(string) string { return "" }

func launchApplication(context.Context, string) error {
	return errors.New("application launch is unavailable on this platform")
}
