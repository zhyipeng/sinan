//go:build !darwin || !cgo

package catalog

import "errors"

func readMetadata(string) (metadata, error) {
	return metadata{}, errors.New("application search requires macOS with cgo")
}

func readIcon(string) string { return "" }

func transliterate(string) string { return "" }
