package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"

	"sinan/internal/plugin/manifest"
)

type PackagePreview struct {
	InstalledFingerprint string `json:"installedFingerprint"`
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Version              string `json:"version"`
	Kind                 string `json:"kind"`
	InstalledVersion     string `json:"installedVersion"`
	Fingerprint          string `json:"fingerprint"`
	Changed              bool   `json:"changed"`
}

// packageFingerprint hashes bounded package content and executable bits, never timestamps.
func packageFingerprint(dir string) (string, error) {
	digest := sha256.New()
	var total int64
	count := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("无法读取插件包")
		}
		info, err := entry.Info()
		if err != nil || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("插件包不能包含符号链接")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.New("插件路径无效")
		}
		if rel == "." {
			if !entry.IsDir() {
				return errors.New("插件入口必须是目录")
			}
			return nil
		}
		count++
		if count > 2048 {
			return errors.New("插件文件数量超过限制")
		}
		if !entry.IsDir() && !info.Mode().IsRegular() {
			return errors.New("插件仅允许普通文件")
		}
		header, _ := json.Marshal([]any{filepath.ToSlash(rel), entry.IsDir(), info.Mode().Perm()&0111 != 0})
		fmt.Fprintf(digest, "%d:", len(header))
		digest.Write(header)
		if entry.IsDir() {
			return nil
		}
		if info.Size() > 64<<20-total {
			return errors.New("插件包超过 64 MiB 限制")
		}
		fmt.Fprintf(digest, "%d:", info.Size())
		file, err := os.Open(path)
		if err != nil {
			return errors.New("无法读取插件文件")
		}
		n, copyErr := io.Copy(digest, io.LimitReader(file, (64<<20)-total+1))
		file.Close()
		total += n
		if copyErr != nil || total > 64<<20 || n != info.Size() {
			return errors.New("插件文件读取失败或正在变化")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func loadPackage(dir string) (manifest.Manifest, string, error) {
	mf, err := manifest.Load(dir)
	if err != nil {
		return mf, "", err
	}
	if mf.Runtime.Kind == "process" {
		if _, err := manifest.Executable(dir, mf); err != nil {
			return mf, "", err
		}
	}
	digest, err := packageFingerprint(dir)
	if err != nil {
		return mf, "", err
	}
	current, err := manifest.Load(dir)
	if err != nil || !reflect.DeepEqual(mf, current) {
		return mf, "", errors.New("插件清单正在变化，请稍后重试")
	}
	return mf, digest, nil
}
func (m *Manager) packageSource(source string) (string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", errors.New("插件目录无效")
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errors.New("插件目录不存在")
	}
	if rel, err := filepath.Rel(abs, m.root); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return "", errors.New("安装来源不能包含插件安装目录")
	}
	return abs, nil
}
func (m *Manager) PreviewPackage(source string) (PackagePreview, error) {
	abs, err := m.packageSource(source)
	if err != nil {
		return PackagePreview{}, err
	}
	mf, digest, err := loadPackage(abs)
	if err != nil {
		return PackagePreview{}, err
	}
	preview := PackagePreview{ID: mf.ID, Name: mf.Name, Version: mf.Version, Kind: mf.Runtime.Kind, Fingerprint: digest, Changed: true}
	m.mu.Lock()
	closed := m.closed
	old := m.plugins[mf.ID]
	m.mu.Unlock()
	if closed {
		return PackagePreview{}, errors.New("运行时已关闭")
	}
	if old != nil {
		preview.InstalledVersion = old.manifest.Version
		preview.InstalledFingerprint = old.fingerprint
		installed, err := packageFingerprint(old.dir)
		if err != nil || installed != old.fingerprint {
			old.markUIChanged()
		}
		preview.Changed = err != nil || installed != digest || old.fingerprint != digest
	}
	return preview, nil
}
