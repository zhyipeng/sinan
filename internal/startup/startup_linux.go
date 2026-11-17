package startup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func newRegistration(executable string) (Registration, error) {
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.New("无法确定开机自启目录")
		}
		config = filepath.Join(home, ".config")
	}
	// Exec quoting and desktop-entry string escaping are separate layers.
	quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`, "%", "%%").Replace(executable)
	quoted = strings.ReplaceAll(quoted, `\`, `\\`)
	content := "[Desktop Entry]\nType=Application\nVersion=1.0\nName=Sinan\nExec=\"" + quoted + "\"\nTerminal=false\n"
	return fileRegistration{filepath.Join(config, "autostart", "dev.sinan.desktop.desktop"), []byte(content)}, nil
}
