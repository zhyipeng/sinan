package pluginmanager

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	"sinan/internal/search"
)

var releaseArchive = regexp.MustCompile(`^(.+)-((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))-(?:darwin|windows|linux)-(?:arm64|amd64)(?i:\.zip)$`)

// MatchLocalArchive supplies a local-install action without accessing the
// filesystem. The existing installer checks the file when the user selects it.
func MatchLocalArchive(plugin kernel.PluginInfo, input string) (search.Item, bool) {
	if plugin.ID != "system.plugin-manager" || !slices.ContainsFunc(plugin.Commands, func(command manifest.Command) bool {
		return command.ID == "open" && command.View == "manager" && command.UIView == "" && !command.SearchOnly
	}) {
		return search.Item{}, false
	}
	path := strings.TrimSpace(input)
	if len(path) >= 2 && (path[0] == '"' || path[0] == '\'') && path[0] == path[len(path)-1] {
		path = path[1 : len(path)-1]
	}
	if !filepath.IsAbs(path) || os.IsPathSeparator(path[len(path)-1]) || strings.ContainsAny(path, "\x00\r\n") {
		return search.Item{}, false
	}
	name := filepath.Base(path)
	parts := releaseArchive.FindStringSubmatch(name)
	if len(parts) == 0 || !manifest.ValidID(parts[1]) || len(parts[2]) > 32 {
		return search.Item{}, false
	}
	return search.Item{
		Key: plugin.ID + "/$local-package/open", PluginID: plugin.ID,
		ProviderID: "$local-package", CommandID: "open", Input: path,
		Title: "安装插件", Subtitle: name, Kind: plugin.Kind, Score: 1,
	}, true
}
