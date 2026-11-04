//go:build darwin && cgo

package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeBundleMetadataAndUserDirectoryWithoutEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Example.app")
	os.MkdirAll(filepath.Join(path, "Contents"), 0700)
	plist := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleIdentifier</key><string>test.sinan.example</string><key>CFBundleDisplayName</key><string>测试应用</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := readMetadata(path)
	if err != nil || meta.DisplayName != "测试应用" {
		t.Fatalf("native metadata: %v", err)
	}
	t.Setenv("HOME", "")
	roots := DefaultRoots()
	if len(roots) != 4 || !filepath.IsAbs(roots[3]) {
		t.Fatal("sanitized environment lost user Applications")
	}
}

func TestNativeLocalizedLoctableAndSymlink(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Localized.app")
	resources := filepath.Join(path, "Contents", "Resources")
	os.MkdirAll(resources, 0700)
	plist := `<plist version="1.0"><dict><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleIdentifier</key><string>test.sinan.localized</string><key>CFBundleDisplayName</key><string>Example</string></dict></plist>`
	os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(plist), 0600)
	table := `<plist version="1.0"><dict><key>zh_CN</key><dict><key>CFBundleDisplayName</key><string>本地化示例</string></dict><key>en</key><dict><key>CFBundleDisplayName</key><string>Example</string></dict></dict></plist>`
	os.WriteFile(filepath.Join(resources, "InfoPlist.loctable"), []byte(table), 0600)
	alias := filepath.Join(root, "Entry.app")
	os.Symlink(path, alias)
	meta, err := readMetadata(alias)
	if err != nil || meta.DisplayName == "" {
		t.Fatalf("symlink metadata: %v", err)
	}
	// The actual name follows the user's system language, never the RPC LANG variable.
	if meta.DisplayName != "Example" && meta.DisplayName != "本地化示例" {
		t.Fatal("unexpected localized title")
	}
}
