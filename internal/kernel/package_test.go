package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func flatPackageFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "example-flat")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	declaration := `{"id":"example.flat","name":"Flat","version":"1.0.0","apiVersion":"1","runtime":{"kind":"declarative"},"contributes":{"commands":[{"id":"hello","title":"Hello","message":"ok"}]},"permissions":[],"activationEvents":["onCommand:hello"]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(declaration), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPackageContentsReportsFlatSnapshotAndLocalBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	manager, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err = manager.Install(flatPackageFixture(t), false); err != nil {
		t.Fatal(err)
	}
	contents, err := manager.PackageContents("example.flat")
	if err != nil {
		t.Fatal(err)
	}
	if !contents.Flat || len(contents.Files) != 1 || contents.Files[0] != "manifest.json" || len(contents.Fingerprint) != 64 {
		t.Fatalf("unexpected contents %+v", contents)
	}
	// A directory is not a shipped snapshot any more, even though its files stay.
	if err = os.Mkdir(filepath.Join(root, "example.flat", "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	if contents, err = manager.PackageContents("example.flat"); err != nil || contents.Flat || len(contents.Files) != 1 {
		t.Fatalf("local directory not reported: %+v %v", contents, err)
	}
	if contents, err = manager.PackageContents("example.missing"); err != nil || contents.Flat || len(contents.Files) != 0 {
		t.Fatalf("missing package not empty: %+v %v", contents, err)
	}
}
