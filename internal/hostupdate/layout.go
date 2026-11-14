// Package hostupdate implements bounded host package staging and replacement.
// Release policy and UI belong to the system settings plugin.
package hostupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"sinan/internal/buildinfo"
)

const MaxArchive = 256 << 20
const maxExpanded = 1 << 30

type Metadata struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

func Platform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}
func Arch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}
func suffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
func ArchiveRoot() string {
	if runtime.GOOS == "darwin" {
		return "Sinan.app"
	}
	return "Sinan-" + Platform() + "-" + Arch()
}
func BinaryDir(target string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(target, "Contents", "MacOS")
	}
	return target
}
func metadataPath(target string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(target, "Contents", "Resources", "release.json")
	}
	return filepath.Join(target, "release.json")
}
func ReadMetadata(target string) (Metadata, error) {
	var meta Metadata
	info, err := os.Lstat(metadataPath(target))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2048 {
		return meta, errors.New("缺少有效发行元数据")
	}
	data, err := os.ReadFile(metadataPath(target))
	if err != nil || json.Unmarshal(data, &meta) != nil || !buildinfo.ValidVersion(meta.Version) || meta.Platform != Platform() || meta.Arch != Arch() {
		return Metadata{}, errors.New("发行版本或平台不匹配")
	}
	return meta, nil
}
func ValidatePackage(target, version string) error {
	meta, err := ReadMetadata(target)
	if err != nil || meta.Version != version {
		return errors.New("发行包元数据与候选版本不符")
	}
	for _, name := range []string{"sinan", "sinanctl", "sinan-ui-host", "sinan-updater"} {
		info, err := os.Lstat(filepath.Join(BinaryDir(target), name+suffix()))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
			return errors.New("发行包缺少可执行文件")
		}
	}
	if runtime.GOOS != "darwin" {
		allowed := map[string]bool{"release.json": true}
		for _, name := range []string{"sinan", "sinanctl", "sinan-ui-host", "sinan-updater"} {
			allowed[name+suffix()] = true
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return errors.New("无法读取便携发行目录")
		}
		for _, entry := range entries {
			if !allowed[entry.Name()] {
				return errors.New("发行目录包含额外文件，请使用独立便携目录更新")
			}
		}
	}
	return nil
}
func Installation(executable, version string) (string, error) {
	exe, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", errors.New("无法确定应用位置")
	}
	if filepath.Base(exe) != "sinan"+suffix() {
		return "", errors.New("仅支持更新已打包的桌面应用")
	}
	target := filepath.Dir(exe)
	if runtime.GOOS == "darwin" {
		if filepath.Base(target) != "MacOS" || filepath.Base(filepath.Dir(target)) != "Contents" {
			return "", errors.New("请从已安装的 Sinan.app 启动")
		}
		target = filepath.Dir(filepath.Dir(target))
		if filepath.Ext(target) != ".app" {
			return "", errors.New("应用目录无效")
		}
	}
	if filepath.Dir(target) == target {
		return "", errors.New("不能替换系统根目录")
	}
	if err := ValidatePackage(target, version); err != nil {
		return "", errors.New("当前运行的是开发构建或非完整发行包")
	}
	return target, nil
}
func AssetName(version string) string {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	return "Sinan-" + version + "-" + osName + "-" + runtime.GOARCH + extension
}
