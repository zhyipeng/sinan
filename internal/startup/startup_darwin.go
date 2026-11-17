package startup

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
)

func newRegistration(executable string) (Registration, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("无法确定开机自启目录")
	}
	var escaped bytes.Buffer
	if xml.EscapeText(&escaped, []byte(executable)) != nil {
		return nil, errors.New("开机自启路径无效")
	}
	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>dev.sinan.desktop</string>
<key>ProgramArguments</key><array><string>` + escaped.String() + `</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>
`
	return fileRegistration{filepath.Join(home, "Library", "LaunchAgents", "dev.sinan.desktop.plist"), []byte(content)}, nil
}
