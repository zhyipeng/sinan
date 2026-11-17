package kernel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"sinan/internal/paste"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

func dependencyPair(t *testing.T) (*Manager, *uiview.Session, string, string) {
	t.Helper()
	m := manager(t)
	targetDir := uiPackage(t, true, "0.3.0")
	mf, err := manifest.Load(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	mf.ID = "example.ocr"
	mf.Contributes.Commands = []manifest.Command{{ID: "open", Title: "OCR", UIView: "main", InputMatchers: []protocol.InputMatcher{{Kind: "image"}}}, {ID: "echo", Title: "Echo", SearchOnly: true}}
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	mf.Contributes.UIViews[0].Commands = []string{"echo"}
	mf.Contributes.EntryPoints = []protocol.EntryPoint{{ID: "recognize-image", Version: "1", CommandID: "open", Input: "image"}}
	mf.ActivationEvents = []string{"onCommand:open"}
	writeManifest(t, targetDir, mf)
	if _, err = m.Install(targetDir, true); err != nil {
		t.Fatal(err)
	}
	sourceDir := uiPackage(t, true, "0.1.0")
	mf, err = manifest.Load(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	mf.Dependencies = []protocol.Dependency{{ID: "ocr", PluginID: "example.ocr", MinVersion: "0.3.0", Optional: true, Entries: []protocol.EntryRequirement{{ID: "recognize-image", Version: "1"}}}}
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	mf.Contributes.UIViews[0].Dependencies = []string{"ocr"}
	writeManifest(t, sourceDir, mf)
	if _, err = m.Install(sourceDir, true); err != nil {
		t.Fatal(err)
	}
	s := openUI(t, m, "example.native")
	s.Message([]byte(`{"kind":"ready","version":2}`))
	return m, s, sourceDir, targetDir
}
func dependencyPNG(t *testing.T) *paste.Content {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	c, err := paste.Parse(protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(b.Bytes())}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestDependencyResolutionAndOptionalIsolation(t *testing.T) {
	m, s, _, _ := dependencyPair(t)
	statuses := callUI(t, s, `{"method":"dependency.status"}`)
	if !bytes.Contains(statuses, []byte(`"available"`)) {
		t.Fatal(string(statuses))
	}
	for _, pair := range [][2]string{{"other", "recognize-image"}, {"ocr", "echo"}} {
		if _, err := m.ResolveDependency(s, pair[0], pair[1]); err == nil {
			t.Fatal("unauthorized target accepted")
		}
	}
	if _, err := m.SetPluginEnabled("example.ocr", false); err != nil {
		t.Fatal(err)
	}
	if !s.Active() {
		t.Fatal("optional dependency revoked source")
	}
	if _, err := m.ResolveDependency(s, "ocr", "recognize-image"); err == nil {
		t.Fatal("disabled target accepted")
	}
	if !bytes.Contains(callUI(t, s, `{"method":"dependency.status"}`), []byte(`"disabled"`)) {
		t.Fatal("wrong status")
	}
	if err := m.Uninstall("example.ocr"); err != nil {
		t.Fatal(err)
	}
	if !s.Active() || !bytes.Contains(callUI(t, s, `{"method":"dependency.status"}`), []byte(`"missing"`)) {
		t.Fatal("missing optional dependency blocked editor")
	}
}
func TestDependencyHandoffActivationAndFailure(t *testing.T) {
	for _, kind := range []string{"commit", "swap-failure", "target-disabled", "source-revoked"} {
		t.Run(kind, func(t *testing.T) {
			m, source, _, _ := dependencyPair(t)
			resolved, err := m.ResolveDependency(source, "ocr", "recognize-image")
			if err != nil {
				t.Fatal(err)
			}
			target, err := m.PrepareDependencyImage(context.Background(), source, "ocr", "recognize-image", resolved.Fingerprint, dependencyPNG(t))
			if err != nil {
				t.Fatal(err)
			}
			target.Message([]byte(`{"kind":"ready","version":2}`))
			done := make(chan error, 1)
			go func() {
				raw, err := target.Call(context.Background(), []byte(`{"method":"launch.get"}`))
				if err == nil {
					var launch protocol.UILaunchContext
					if json.Unmarshal(raw, &launch) != nil || launch.Image == nil {
						err = errors.New("missing transferred image")
					}
				}
				done <- err
			}()
			select {
			case <-done:
				t.Fatal("target business started before commit")
			case <-time.After(30 * time.Millisecond):
			}
			if kind == "target-disabled" {
				m.SetPluginEnabled("example.ocr", false)
			}
			if kind == "source-revoked" {
				source.Revoke()
			}
			swapped := false
			err = m.CommitDependency(context.Background(), source, target, "ocr", "recognize-image", func() error {
				if kind == "swap-failure" {
					return errors.New("renderer failed")
				}
				swapped = true
				return nil
			})
			if kind == "commit" {
				if err != nil || !swapped {
					t.Fatal(err)
				}
				source.Revoke()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if !target.Active() {
					t.Fatal("source close cancelled target")
				}
			} else {
				if err == nil || swapped {
					t.Fatal("stale handoff committed")
				}
				target.Revoke()
				if err := <-done; err == nil {
					t.Fatal("failed target activated")
				}
				if kind != "source-revoked" && !source.Active() {
					t.Fatal("failed handoff lost source")
				}
			}
		})
	}
}
func TestRequiredDependencyCyclesAndRevocation(t *testing.T) {
	m, source, sourceDir, targetDir := dependencyPair(t)
	source.Revoke()
	mf, _ := manifest.Load(sourceDir)
	mf.Version = "0.2.0"
	mf.Dependencies[0].Optional = false
	writeManifest(t, sourceDir, mf)
	if _, err := m.Upgrade(sourceDir, true); err != nil {
		t.Fatal(err)
	}
	source = openUI(t, m, "example.native")
	target, _ := manifest.Load(targetDir)
	target.Version = "0.4.0"
	target.Dependencies = []protocol.Dependency{{ID: "back", PluginID: "example.native", MinVersion: "0.1.0", Entries: []protocol.EntryRequirement{{ID: "missing", Version: "1"}}}}
	writeManifest(t, targetDir, target)
	if _, err := m.Upgrade(targetDir, true); err == nil {
		t.Fatal("hard dependency cycle installed")
	}
	if _, err := m.SetPluginEnabled("example.ocr", false); err != nil {
		t.Fatal(err)
	}
	if source.Active() {
		t.Fatal("hard-dependent live session survived disable")
	}
	if _, err := m.Execute(context.Background(), "example.native", "echo", ""); err == nil {
		t.Fatal("hard-dependent command still executed")
	}
	if _, err := m.OpenUIView(context.Background(), "example.native", "main"); err == nil {
		t.Fatal("missing hard dependency activated source")
	}
}

func TestDependencyRejectsOldAndMissingExport(t *testing.T) {
	m, s, _, targetDir := dependencyPair(t)
	for _, kind := range []string{"old-version", "missing-export"} {
		mf, err := manifest.Load(targetDir)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "old-version" {
			mf.Version = "0.2.0"
		} else {
			mf.Version = "0.3.0"
			mf.Contributes.EntryPoints = nil
		}
		writeManifest(t, targetDir, mf)
		if _, err = m.Upgrade(targetDir, true); err != nil {
			t.Fatal(err)
		}
		if _, err = m.ResolveDependency(s, "ocr", "recognize-image"); err == nil {
			t.Fatal("incompatible target accepted")
		}
		if !s.Active() {
			t.Fatal("optional incompatibility revoked source")
		}
	}
}
