package scaffold

import (
	"archive/zip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
)

func TestGeneratedProjectsBuildAndInstall(t *testing.T) {
	for _, kind := range []string{"html", "go-html"} {
		t.Run(kind, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "插件 projects")
			if err := os.Mkdir(parent, 0755); err != nil {
				t.Fatal(err)
			}
			result, err := Create(Options{Name: "工具 <script> \"示例\"", ID: "local.example", Template: kind, Directory: parent})
			if err != nil {
				t.Fatal(err)
			}
			installFrontend(t, result.Directory)
			cmd := exec.Command("node", "scripts/build.mjs")
			cmd.Dir = result.Directory
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("standalone build failed: %v\n%s", err, output)
			}
			pack := exec.Command("python3", "scripts/package.py", "v0.1.0")
			pack.Dir = result.Directory
			pack.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
			if out, err := pack.CombinedOutput(); err != nil {
				t.Fatalf("package failed: %v %s", err, out)
			}
			archive := filepath.Join(result.Directory, "dist", "release", "local.example-0.1.0-"+runtime.GOOS+"-"+runtime.GOARCH+".zip")
			zipped, err := zip.OpenReader(archive)
			if err != nil {
				t.Fatal(err)
			}
			foundManifest := false
			for _, file := range zipped.File {
				if file.Name == "manifest.json" {
					foundManifest = true
				}
				if file.Name == "AGENTS.md" || strings.HasPrefix(file.Name, ".agents/") || strings.HasPrefix(file.Name, ".github/") {
					t.Fatal("developer files leaked into runtime package")
				}
			}
			zipped.Close()
			if !foundManifest {
				t.Fatal("archive has no root manifest")
			}
			mf, err := manifest.Load(result.PackageDirectory)
			if err != nil || mf.Name != "工具 <script> \"示例\"" {
				t.Fatalf("invalid generated manifest: %v", err)
			}
			if _, err = os.Stat(filepath.Join(result.Directory, "go.mod")); (kind == "html") != os.IsNotExist(err) {
				t.Fatal("wrong runtime scaffold")
			}
			manager, err := kernel.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			if _, err = manager.Install(result.PackageDirectory, kind == "go-html"); err != nil {
				t.Fatal("install generated package", err)
			}
			if kind == "go-html" {
				response, err := manager.Execute(context.Background(), "local.example", "echo", "独立后端 ready")
				if err != nil || response.Text != "独立后端 ready" {
					t.Fatalf("generated backend failed: %v", err)
				}
			}
			session, err := manager.OpenUICommand(context.Background(), "local.example", "open")
			if err != nil {
				t.Fatal("prepare generated UI without opening a window", err)
			}
			if !session.Active() {
				t.Fatal("generated UI session inactive")
			}
		})
	}
}

func TestRejectsInvalidProjectsWithoutWriting(t *testing.T) {
	for _, change := range []func(*Options){
		func(o *Options) { o.ID = "../escape" },
		func(o *Options) { o.ID = "system.settings" },
		func(o *Options) { o.ID = "a/b" },
		func(o *Options) { o.Template = "unknown" },
		func(o *Options) { o.Name = " \n " },
		func(o *Options) { o.Name = "a\nb" },
		func(o *Options) { o.Name = strings.Repeat("中", 65) },
		func(o *Options) { o.Directory = "relative" },
	} {
		parent := t.TempDir()
		o := Options{Name: "插件", ID: "local.valid", Template: "html", Directory: parent}
		change(&o)
		if _, err := Create(o); err == nil {
			t.Fatal("invalid project accepted")
		}
		entries, _ := os.ReadDir(parent)
		if len(entries) != 0 {
			t.Fatal("invalid request wrote files")
		}
	}
}

func TestExistingDirectoriesAndLinksArePreserved(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "local.existing")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	o := Options{Name: "已有项目", ID: "local.existing", Template: "html", Directory: parent}
	if _, err := Create(o); err == nil {
		t.Fatal("overwrote existing project")
	}
	if data, _ := os.ReadFile(sentinel); string(data) != "keep" {
		t.Fatal("existing file changed")
	}
	if err := os.Symlink(target, filepath.Join(parent, "local.link")); err != nil {
		t.Skip("symlink unavailable")
	}
	o.ID = "local.link"
	if _, err := Create(o); err == nil {
		t.Fatal("reused existing symlink")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 1 {
		t.Fatal("wrote through preexisting symlink")
	}
}

func TestDevelopmentBuildCreatesReusableLinkAndPreservesConflicts(t *testing.T) {
	parent := t.TempDir()
	install := t.TempDir()
	project, err := Create(Options{Name: "开发工具", ID: "local.dev", Template: "html", Directory: parent})
	if err != nil {
		t.Fatal(err)
	}
	installFrontend(t, project.Directory)
	build := func(wantSuccess bool) {
		t.Helper()
		cmd := exec.Command("node", "scripts/build.mjs", "--dev", "--plugins-dir", install)
		cmd.Dir = project.Directory
		out, err := cmd.CombinedOutput()
		if (err == nil) != wantSuccess {
			t.Fatalf("dev build: %v %s", err, out)
		}
	}
	build(true)
	build(true)
	link := filepath.Join(install, "local.dev")
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("no development symlink", err)
	}
	manager, err := kernel.New(install)
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.Snapshot().Plugins) != 1 || !manager.Snapshot().Plugins[0].Development {
		t.Fatal("linked template not loaded")
	}
	if err = manager.Uninstall("local.dev"); err != nil {
		t.Fatal(err)
	}
	manager.Close()
	if _, err = os.Stat(project.PackageDirectory); err != nil {
		t.Fatal("external package was removed")
	}
	if err = os.Mkdir(link, 0755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(link, "keep.txt")
	if err = os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	build(false)
	if bytes, _ := os.ReadFile(sentinel); string(bytes) != "keep" {
		t.Fatal("existing install overwritten")
	}
}

func installFrontend(t *testing.T, directory string) {
	t.Helper()
	cmd := exec.Command("npm", "ci", "--no-audit", "--no-fund", "--prefer-offline")
	cmd.Dir = directory
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("frontend dependency install failed: %v %s", err, out)
	}
}
