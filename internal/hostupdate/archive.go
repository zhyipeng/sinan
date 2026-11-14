package hostupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type extractor struct {
	destination string
	seen        map[string]bool
	paths       map[string]string
	count       int
	total       int64
}

func (e *extractor) entry(name string, directory bool, mode os.FileMode, size int64, source io.Reader) error {
	// Generated packages contain no links. Reject all path aliases, including
	// Windows ADS, reserved names and case collisions, on every platform.
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.ContainsAny(name, "\\:\x00") || path.IsAbs(name) || path.Clean(name) != name {
		return errors.New("归档路径无效")
	}
	parts := strings.Split(name, "/")
	if parts[0] != ArchiveRoot() {
		return errors.New("归档根目录不匹配")
	}
	for _, part := range parts {
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if part == "." || part == ".." || strings.TrimRight(part, ". ") != part || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return errors.New("归档包含不支持的路径")
		}
	}
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		key := strings.ToLower(prefix)
		if previous, exists := e.paths[key]; exists && previous != prefix {
			return errors.New("归档包含大小写目录冲突")
		}
		e.paths[key] = prefix
	}
	key := strings.ToLower(name)
	e.count++
	if e.count > 4096 || e.seen[key] || size < 0 || size > 512<<20 {
		return errors.New("归档文件数量、大小或路径冲突")
	}
	e.seen[key] = true
	e.total += size
	if e.total > maxExpanded {
		return errors.New("归档解压预算超限")
	}
	target := filepath.Join(e.destination, filepath.FromSlash(name))
	if directory {
		return os.MkdirAll(target, 0755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	// Discard setuid/setgid and writable-by-others bits; preserve execute bits.
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644|mode.Perm()&0111)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(file, io.LimitReader(source, size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != size {
		return errors.New("归档文件长度不符")
	}
	return nil
}

// Extract writes only into a newly-created private directory supplied by caller.
func Extract(archive, destination string, zipFormat bool) error {
	e := extractor{destination: destination, seen: map[string]bool{}, paths: map[string]string{}}
	if zipFormat {
		z, err := zip.OpenReader(archive)
		if err != nil {
			return errors.New("ZIP 格式无效")
		}
		defer z.Close()
		for _, file := range z.File {
			if !file.FileInfo().IsDir() && !file.Mode().IsRegular() {
				return errors.New("归档不允许链接或特殊文件")
			}
			reader, err := file.Open()
			if err != nil {
				return err
			}
			err = e.entry(file.Name, file.FileInfo().IsDir(), file.Mode(), int64(file.UncompressedSize64), reader)
			reader.Close()
			if err != nil {
				return err
			}
		}
	} else {
		file, err := os.Open(archive)
		if err != nil {
			return err
		}
		defer file.Close()
		gz, err := gzip.NewReader(file)
		if err != nil {
			return errors.New("gzip 格式无效")
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return errors.New("tar 格式无效")
			}
			if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
				return errors.New("归档不允许链接或特殊文件")
			}
			if err = e.entry(header.Name, header.Typeflag == tar.TypeDir, os.FileMode(header.Mode), header.Size, tr); err != nil {
				return err
			}
		}
	}
	if e.count == 0 {
		return errors.New("发行归档为空")
	}
	return nil
}
