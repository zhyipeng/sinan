//go:build !darwin || !cgo

package searchtext

func transliterate(string) string { return "" }
