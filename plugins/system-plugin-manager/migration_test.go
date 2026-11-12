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
		if plugin.ID != "system.plugin-manager" || plugin.Name != "插件中心" || plugin.Version != "0.7.0" || !plugin.Protected {
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
