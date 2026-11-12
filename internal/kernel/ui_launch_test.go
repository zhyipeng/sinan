package kernel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"sinan/internal/paste"
	"sinan/internal/plugin/manifest"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
)

func launchFixture(t *testing.T, m *Manager, id string) {
	t.Helper()
	src := uiPackage(t, false, "0.1.0")
	mf, err := manifest.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	mf.ID = id
	mf.Contributes.Commands = []manifest.Command{{ID: "open", Title: "Open", UIView: "main"}}
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	writeManifest(t, src, mf)
	if _, err = m.Install(src, false); err != nil {
		t.Fatal(err)
	}
}
func readLaunchForTest(t *testing.T, s *uiview.Session) protocol.UILaunchContext {
	t.Helper()
	if _, err := s.Message([]byte(`{"kind":"ready","version":2}`)); err != nil {
		t.Fatal(err)
	}
	var launch protocol.UILaunchContext
	if err := json.Unmarshal(callUI(t, s, `{"method":"launch.get"}`), &launch); err != nil {
		t.Fatal(err)
	}
	return launch
}
func TestLaunchContextIsSessionScopedAndReadOnly(t *testing.T) {
	m := manager(t)
	launchFixture(t, m, "example.first")
	launchFixture(t, m, "example.second")
	input := " \n {\"big\":900719925474099312345,\"str\":\"</script><>&\"} \t"
	first, err := m.OpenUICommandWithInput(context.Background(), "example.first", "open", input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.OpenUICommand(context.Background(), "example.second", "open")
	if err != nil {
		t.Fatal(err)
	}
	launch := readLaunchForTest(t, first)
	if launch.Input == nil || *launch.Input != input || launch.CommandID != "open" {
		t.Fatal("launch context lost original text")
	}
	if readLaunchForTest(t, second).Input != nil {
		t.Fatal("launch leaked between plugins")
	}
	plain, err := m.OpenUICommand(context.Background(), "example.first", "open")
	if err != nil {
		t.Fatal(err)
	}
	if readLaunchForTest(t, plain).Input != nil {
		t.Fatal("normal opening reused old input")
	}
	for _, req := range []string{`{"method":"launch.get","pluginId":"example.first"}`, `{"method":"launch.get","input":"replace"}`, `{"method":"launch.get","commandId":"open"}`, `{"method":"launch.get","value":null}`, `{"method":"launch.get","method":"launch.get"}`} {
		if _, err := second.Call(context.Background(), []byte(req)); err == nil {
			t.Fatal("unscoped launch request accepted")
		}
	}
	if _, err := first.Call(context.Background(), []byte(`{"method":"storage.get"}`)); err == nil {
		t.Fatal("reading launch granted storage")
	}
	if _, err := m.SetPluginEnabled("example.first", false); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Call(context.Background(), []byte(`{"method":"launch.get"}`)); err == nil {
		t.Fatal("revoked session retained launch access")
	}
}
func TestLaunchInputLimitsAndEmptyInput(t *testing.T) {
	m := manager(t)
	launchFixture(t, m, "example.first")
	for _, input := range []string{strings.Repeat("x", (64<<10)+1), strings.Repeat("\x00", 64<<10), string([]byte{0xff})} {
		if _, err := m.OpenUICommandWithInput(context.Background(), "example.first", "open", input); err == nil {
			t.Fatal("invalid or oversized launch accepted")
		}
	}
	for _, input := range []string{"", strings.Repeat("x", 64<<10)} {
		s, err := m.OpenUICommandWithInput(context.Background(), "example.first", "open", input)
		if err != nil {
			t.Fatal(err)
		}
		launch := readLaunchForTest(t, s)
		if launch.Input == nil || *launch.Input != input {
			t.Fatal("explicit input changed")
		}
		s.Revoke()
	}
	// Bridge 1 is unchanged and cannot consume launch data.
	src := uiPackage(t, false, "0.2.0")
	mf, _ := manifest.Load(src)
	mf.ID = "example.first"
	mf.Contributes.Commands = []manifest.Command{{ID: "open", Title: "Open", UIView: "main"}}
	writeManifest(t, src, mf)
	if _, err := m.Upgrade(src, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenUICommandWithInput(context.Background(), "example.first", "open", "{}"); err == nil {
		t.Fatal("bridge 1 accepted launch input")
	}
}

func TestPastedImageIsIsolatedAndRevoked(t *testing.T) {
	m := manager(t)
	src := uiPackage(t, false, "0.1.0")
	mf, _ := manifest.Load(src)
	mf.Contributes.Commands = []manifest.Command{{ID: "open", Title: "Image", UIView: "main", InputMatchers: []protocol.InputMatcher{{Kind: "image"}}}}
	mf.Contributes.UIViews[0].BridgeVersion = "2"
	writeManifest(t, src, mf)
	if _, err := m.Install(src, false); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	content, err := paste.Parse(protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(buf.Bytes())}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.OpenUICommandWithPaste(context.Background(), mf.ID, "open", content)
	if err != nil {
		t.Fatal(err)
	}
	launch := readLaunchForTest(t, first)
	if launch.Input != nil || launch.Image == nil || launch.Image.Width != 2 || launch.Image.Height != 3 || launch.Image.MIMEType != "image/png" {
		t.Fatal("wrong image opening data")
	}
	resource, ok := first.Resource(launch.Image.URL)
	if !ok || !bytes.Equal(resource.Data, buf.Bytes()) {
		t.Fatal("image bytes not available in owner session")
	}
	second, err := m.OpenUICommandWithPaste(context.Background(), mf.ID, "open", content)
	if err != nil {
		t.Fatal(err)
	}
	secondLaunch := readLaunchForTest(t, second)
	if secondLaunch.Image.URL == launch.Image.URL {
		t.Fatal("image URL reused across sessions")
	}
	if _, ok := second.Resource(launch.Image.URL); ok {
		t.Fatal("another session read the image")
	}
	plain, err := m.OpenUICommand(context.Background(), mf.ID, "open")
	if err != nil {
		t.Fatal(err)
	}
	if readLaunchForTest(t, plain).Image != nil {
		t.Fatal("normal open reused clipboard image")
	}
	first.Revoke()
	if _, ok := first.Resource(launch.Image.URL); ok {
		t.Fatal("revoked image remained accessible")
	}
	text := "{}"
	textContent, _ := paste.Parse(protocol.PasteContent{Text: &text})
	if _, err := m.OpenUICommandWithPaste(context.Background(), mf.ID, "open", textContent); err == nil {
		t.Fatal("image-only command accepted text")
	}
}
