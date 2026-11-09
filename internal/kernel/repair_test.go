package kernel

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sinan/internal/plugin/manifest"
)

func systemSource(t *testing.T, id string) string {
	t.Helper()
	dir := t.TempDir()
	name := map[string]string{"system.settings": "system-settings", "system.plugin-manager": "system-plugin-manager"}[id]
	if err := copyDirectory(filepath.Join("../../plugins", name), dir); err != nil {
		t.Fatal(err)
	}
	return dir
}
func applyRepair(t *testing.T, m *Manager, source string) PluginInfo {
	t.Helper()
	preview, err := m.PreviewSystemRepair(source)
	if err != nil {
		t.Fatal(err)
	}
	info, err := m.RepairSystemPlugin(source, preview.Fingerprint, preview.InstalledFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
func TestSystemRepairDamagedManifest(t *testing.T) {
	for _, id := range []string{"system.settings", "system.plugin-manager"} {
		for _, restart := range []bool{false, true} {
			t.Run(id+map[bool]string{true: "/restart", false: "/live"}[restart], func(t *testing.T) {
				root := t.TempDir()
				m, err := New(root)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { m.Close() }()
				src := systemSource(t, id)
				applyRepair(t, m, src) // Missing installation also uses the repair entry point.
				dest := filepath.Join(root, id, "manifest.json")
				if err = os.WriteFile(dest, []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
				if restart {
					m.Close()
					m, err = New(root)
					if err != nil {
						t.Fatal(err)
					}
				}
				info := applyRepair(t, m, src)
				if info.ID != id || info.Changed || !info.Protected {
					t.Fatal("invalid repaired instance")
				}
				if _, err = manifest.Load(filepath.Dir(dest)); err != nil {
					t.Fatal(err)
				}
				if _, err = m.SetPluginEnabled(id, false); err == nil {
					t.Fatal("disabled protected plugin")
				}
				if err = m.Uninstall(id); err == nil {
					t.Fatal("uninstalled protected plugin")
				}
				preview, err := m.PreviewSystemRepair(src)
				if err != nil || preview.Changed {
					t.Fatal("healthy package still needs repair", err)
				}
			})
		}
	}
}
func TestSystemRepairBindsBothReviewedPackages(t *testing.T) {
	for _, which := range []string{"candidate", "installed"} {
		t.Run(which, func(t *testing.T) {
			m := manager(t)
			src := systemSource(t, "system.settings")
			applyRepair(t, m, src)
			preview, err := m.PreviewSystemRepair(src)
			if err != nil {
				t.Fatal(err)
			}
			dir := src
			if which == "installed" {
				dir = filepath.Join(m.root, preview.ID)
			}
			if err = os.WriteFile(filepath.Join(dir, "changed"), []byte("change"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = m.RepairSystemPlugin(src, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
				t.Fatal("stale review accepted")
			}
		})
	}
}
func TestSystemRepairRejectsOtherPackages(t *testing.T) {
	m := manager(t)
	if _, err := m.PreviewSystemRepair(source(t, false)); err == nil {
		t.Fatal("ordinary plugin accepted")
	}
	src := systemSource(t, "system.settings")
	mf, _ := manifest.Load(src)
	mf.Contributes.Commands[0].View = "manager"
	writeManifest(t, src, mf)
	if _, err := m.PreviewSystemRepair(src); err == nil {
		t.Fatal("wrong component accepted")
	}
}
func TestSystemRepairRetainsFailedRollbackForRestart(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { m.Close() }()
	src := systemSource(t, "system.settings")
	applyRepair(t, m, src)
	dest := filepath.Join(root, "system.settings", "manifest.json")
	if err = os.WriteFile(dest, []byte("original damaged bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.PreviewSystemRepair(src)
	if err != nil {
		t.Fatal(err)
	}
	// A nonempty approval directory forces approval removal and rollback to fail.
	fault := filepath.Join(root, ".trusted", "system.settings")
	if err = os.MkdirAll(fault, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fault, "block"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.RepairSystemPlugin(src, preview.Fingerprint, preview.InstalledFingerprint); err == nil {
		t.Fatal("fault ignored")
	}
	bytes, err := os.ReadFile(dest)
	if err != nil || string(bytes) != "original damaged bytes" {
		t.Fatal("original files lost", err)
	}
	if !m.packageChangesBlocked("system.settings") {
		t.Fatal("unrecovered journal not blocked")
	}
	m.Close()
	if err = os.RemoveAll(fault); err != nil {
		t.Fatal(err)
	}
	m, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	if m.packageChangesBlocked("system.settings") {
		t.Fatal("startup did not recover journal")
	}
	applyRepair(t, m, src)
	entries, _ := filepath.Glob(filepath.Join(root, ".upgrade-*"))
	if len(entries) != 0 {
		t.Fatal("journal retained after recovery")
	}
}

func TestSystemRepairPreservesHostDataAndRejectsSymlink(t *testing.T) {
	m := manager(t)
	src := systemSource(t, "system.settings")
	applyRepair(t, m, src)
	before, err := m.SaveWindowPreferences(0, "Ctrl+Alt+U")
	if err != nil {
		t.Fatal(err)
	}
	m.usage.Record("system.settings", "open", "fixture")
	dir := filepath.Join(m.root, "system.settings")
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	applyRepair(t, m, src)
	after, err := m.WindowPreferences()
	if err != nil || !reflect.DeepEqual(before, after) || m.UsageBoost("system.settings", "open", "fixture") <= 0 {
		t.Fatal("host data lost", err)
	}
	previous := filepath.Join(t.TempDir(), "previous")
	if err = os.Rename(dir, previous); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(previous, dir); err != nil {
		t.Fatal(err)
	}
	if _, err = m.PreviewSystemRepair(src); err == nil {
		t.Fatal("symlink destination accepted")
	}
	if _, err = manifest.Load(previous); err != nil {
		t.Fatal("linked directory mutated", err)
	}
}
