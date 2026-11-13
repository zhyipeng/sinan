package catalog

import (
	"context"
	"os"
	"os/exec"
)

func readMetadata(path string) (metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return metadata{}, err
	}
	defer f.Close()
	locale := ""
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = os.Getenv(key); locale != "" {
			break
		}
	}
	return desktopMetadata(f, locale, os.Getenv("XDG_CURRENT_DESKTOP"), exec.LookPath)
}

func readIcon(string) string { return "" }

func launchApplication(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "gio", "launch", path).Run()
}
