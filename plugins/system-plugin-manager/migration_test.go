package pluginmanager

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sinan/internal/kernel"
)

func fixtureManager(t *testing.T, ids ...string) (*kernel.Manager, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "plugins")
	manager, err := kernel.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	for _, id := range ids {
		if _, err := manager.Install(filepath.Join("testdata", id), false); err != nil {
			t.Fatal(err)
		}
	}
	return manager, root
}

func TestMigrationMergesIdentityAndPreservesPrivateState(t *testing.T) {
	manager, root := fixtureManager(t, "system-plugin-manager", "system-plugin-store")
	if _, err := manager.SetPluginEnabled("system.plugin-store", false); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(root, ".sinan-state", "settings.json")
	settingsBefore, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, ".sinan-state", "data", "retained.json")
	if err := os.MkdirAll(filepath.Dir(data), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte(`{"retained":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Migrate(manager); err != nil {
			t.Fatal(err)
		}
		snapshot := manager.Snapshot()
		if len(snapshot.Plugins) != 1 {
			t.Fatalf("expected one plugin, got %d", len(snapshot.Plugins))
		}
		plugin := snapshot.Plugins[0]
		if plugin.ID != "system.plugin-manager" || plugin.Name != "插件中心" || plugin.Version != "0.9.3" || !plugin.Protected {
			t.Fatal("invalid merged identity")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "system.plugin-store")); !os.IsNotExist(err) {
		t.Fatal("legacy package remains")
	}
	if content, err := os.ReadFile(data); err != nil || string(content) != `{"retained":true}` {
		t.Fatal("private state changed")
	}
	if settingsAfter, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(settingsBefore, settingsAfter) {
		t.Fatal("settings changed during migration")
	}
	if _, err := manager.SetPluginEnabled("system.plugin-manager", false); err == nil {
		t.Fatal("center can be disabled")
	}
	if err := manager.Uninstall("system.plugin-manager"); err == nil {
		t.Fatal("center can be uninstalled")
	}
}

func TestMigrationPreservesUnknownPackageBytesAcrossRestart(t *testing.T) {
	for _, id := range []string{"system.plugin-manager", "system.plugin-store"} {
		t.Run(id, func(t *testing.T) {
			manager, root := fixtureManager(t, "system-plugin-manager", "system-plugin-store")
			marker := filepath.Join(root, id, "local-change.txt")
			if err := os.WriteFile(marker, []byte("local change"), 0600); err != nil {
				t.Fatal(err)
			}
			manager.Close()
			reopened, err := kernel.New(root)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err := Migrate(reopened); err == nil {
				t.Fatal("unknown package accepted")
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("local file lost")
			}
			if _, err := os.Stat(filepath.Join(root, "system.plugin-store")); err != nil {
				t.Fatal("legacy store removed before safe migration")
			}
		})
	}
}

func TestMigrationFromStoreOnlyAndEmptyInstallation(t *testing.T) {
	empty, _ := fixtureManager(t)
	if err := Migrate(empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.Snapshot().Plugins) != 0 {
		t.Fatal("empty installation was changed")
	}
	storeOnly, _ := fixtureManager(t, "system-plugin-store")
	if err := Migrate(storeOnly); err != nil {
		t.Fatal(err)
	}
	if plugins := storeOnly.Snapshot().Plugins; len(plugins) != 1 || plugins[0].ID != "system.plugin-manager" {
		t.Fatal("store-only installation not migrated")
	}
}

// A release installs only the flat manifest/icon snapshot, so a host upgrade has
// to adopt the bundled declaration instead of reporting an unknown built-in
// package. The fixture is the snapshot released as plugin center 0.9.2.
func TestMigrationAdoptsBundledCenterFromReleasedSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	manager, err := kernel.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err = manager.Install(filepath.Join("testdata", "released-0.9.2"), false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = Migrate(manager); err != nil {
			t.Fatal(err)
		}
		plugins := manager.Snapshot().Plugins
		if len(plugins) != 1 || plugins[0].ID != "system.plugin-manager" || plugins[0].Version != "0.9.3" {
			t.Fatalf("released snapshot not adopted: %+v", plugins)
		}
	}
	contents, err := manager.PackageContents("system.plugin-manager")
	if err != nil || !contents.Flat || len(contents.Files) != 2 {
		t.Fatalf("unexpected installed package %+v %v", contents, err)
	}
}

// Only the center's own declaration may be replaced. Another declarative package
// that happens to use the same two-file layout stays untouched.
func TestMigrationKeepsFlatPackageWithForeignDeclaration(t *testing.T) {
	source := filepath.Join(t.TempDir(), "package")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	declaration := `{"id":"system.plugin-manager","name":"插件中心","version":"0.9.2","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"open","title":"插件中心","view":"store"}]},"permissions":[],"activationEvents":["onCommand:open"]}`
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(declaration), 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "plugins")
	manager, err := kernel.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err = manager.Install(source, false); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(manager); err == nil {
		t.Fatal("foreign declaration accepted")
	}
	plugins := manager.Snapshot().Plugins
	if len(plugins) != 1 || plugins[0].Version != "0.9.2" {
		t.Fatal("foreign package replaced")
	}
}

func TestCheckedUninstallRejectsPackageChangedAfterPreview(t *testing.T) {
	manager, root := fixtureManager(t, "system-plugin-store")
	preview, err := manager.PreviewPackage(filepath.Join("testdata", "system-plugin-store"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "system.plugin-store", "changed.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.UninstallChecked("system.plugin-store", preview.InstalledFingerprint); err == nil {
		t.Fatal("changed package removed")
	}
	if len(manager.Snapshot().Plugins) != 1 {
		t.Fatal("changed package unregistered")
	}
}

// An unsuccessful center upgrade must never remove the old working identity.
type failingMigration struct{ MigrationBackend }

func (f failingMigration) ApplyPackage(string, bool, string, string) (kernel.PluginInfo, error) {
	return kernel.PluginInfo{}, errors.New("simulated apply failure")
}
func TestMigrationKeepsStoreWhenCenterUpgradeFails(t *testing.T) {
	manager, root := fixtureManager(t, "system-plugin-manager", "system-plugin-store")
	if err := Migrate(failingMigration{manager}); err == nil {
		t.Fatal("failed upgrade accepted")
	}
	if len(manager.Snapshot().Plugins) != 2 {
		t.Fatal("legacy plugin unregistered")
	}
	if _, err := os.Stat(filepath.Join(root, "system.plugin-store")); err != nil {
		t.Fatal("store removed after failed center upgrade")
	}
	if err := Migrate(manager); err != nil {
		t.Fatal("retry failed", err)
	}
}
