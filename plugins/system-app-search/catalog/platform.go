package catalog

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultRoots(platform string, getenv func(string) string, homeDir func() (string, error)) []string {
	home, _ := homeDir()
	var roots []string
	switch platform {
	case "darwin":
		roots = []string{"/Applications", "/System/Applications", "/System/Library/CoreServices/Applications"}
		if home != "" {
			roots = append(roots, filepath.Join(home, "Applications"))
		}
	case "windows":
		for _, key := range []string{"APPDATA", "PROGRAMDATA"} {
			if base := getenv(key); filepath.IsAbs(base) {
				roots = append(roots, filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs"))
			}
		}
	case "linux":
		base := getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(base) && home != "" {
			base = filepath.Join(home, ".local", "share")
		}
		if filepath.IsAbs(base) {
			roots = append(roots, filepath.Join(base, "applications"))
		}
		dirs := getenv("XDG_DATA_DIRS")
		if dirs == "" {
			dirs = "/usr/local/share:/usr/share"
		}
		for _, dir := range strings.Split(dirs, ":") {
			if filepath.IsAbs(dir) {
				roots = append(roots, filepath.Join(dir, "applications"))
			}
		}
	}
	return roots
}

func applicationEntry(platform, path string, directory bool) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch platform {
	case "darwin":
		return ext == ".app" // Includes bundle symlinks, validated after resolution.
	case "windows":
		return !directory && (ext == ".lnk" || ext == ".exe")
	case "linux":
		return !directory && ext == ".desktop"
	}
	return false
}

func validApplicationType(platform, path string, info os.FileInfo) bool {
	if platform == "darwin" {
		return info.IsDir() && applicationEntry(platform, path, true)
	}
	return info.Mode().IsRegular() && applicationEntry(platform, path, false)
}

func supportedApplicationPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".app", ".lnk", ".exe", ".desktop":
		return true
	}
	return false
}
