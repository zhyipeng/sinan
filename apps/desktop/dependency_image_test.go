package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/wailsapp/wails/v3/pkg/application"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sinan/internal/kernel"
	"sinan/internal/plugin/manifest"
	protocol "sinan/packages/plugin-protocol"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"sinan/internal/plugin/uiview"
)

func uploadSession(t *testing.T) *uiview.Session {
	bundle, err := uiview.LoadFS(context.Background(), fstest.MapFS{"index.html": {Data: []byte("fixture")}}, "example.upload", "index.html")
	if err != nil {
		t.Fatal(err)
	}
	s, err := uiview.NewSession(uiview.Binding{PluginID: "example.upload", ViewID: "main", BridgeVersion: "2", Fingerprint: strings.Repeat("a", 64)}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Revoke)
	return s
}
func TestDependencyImageRejectsReplayAndInvalidData(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 4)))
	data := b.Bytes()
	owner := uploadSession(t)
	u := newDependencyImage(owner, "ocr", "recognize-image", "fingerprint", len(data))
	if err := u.append(u.id + ":1:" + base64.StdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("out-of-order block accepted")
	}
	if err := u.append(u.id + ":0:" + base64.StdEncoding.EncodeToString(data)); err != nil {
		t.Fatal(err)
	}
	if err := u.finish(u.id); err != nil {
		t.Fatal(err)
	}
	if u.content == nil {
		t.Fatal("PNG not validated")
	}
	if err := u.append(u.id + ":0:" + base64.StdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("finished upload mutated")
	}
	u.close()
	if err := u.finish(u.id); err == nil {
		t.Fatal("consumed upload replayed")
	}
	bad := newDependencyImage(owner, "ocr", "recognize-image", "fingerprint", 3)
	bad.append(bad.id + ":0:YmFk")
	if err := bad.finish(bad.id); err == nil {
		t.Fatal("invalid PNG accepted")
	}
	expired := newDependencyImage(owner, "ocr", "recognize-image", "fingerprint", 3)
	expired.mu.Lock()
	expired.touched = time.Now().Add(-16 * time.Second)
	expired.mu.Unlock()
	if err := expired.append(expired.id + ":0:YmFk"); err == nil {
		t.Fatal("expired upload accepted")
	}
	pending := newDependencyImage(owner, "ocr", "recognize-image", "fingerprint", 3)
	owner.Revoke()
	deadline := time.Now().Add(time.Second)
	for {
		pending.mu.Lock()
		closed := pending.closed
		pending.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload retained after revoke")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDependencyHostUploadIsolationQuotaAndTargetReplacement(t *testing.T) {
	m, err := kernel.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	install := func(id string, target bool) string {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "ui"), 0700); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "ui/index.html"), []byte("fixture"), 0600)
		mf := manifest.Manifest{ID: id, Name: id, Version: "0.3.0", APIVersion: "1", Runtime: manifest.Runtime{Kind: "declarative"}, Contributes: manifest.Contributions{Commands: []manifest.Command{{ID: "open", Title: "Open", UIView: "main"}}, UIViews: []protocol.UIView{{ID: "main", Entry: "index.html", BridgeVersion: "2"}}}}
		if target {
			mf.Contributes.Commands[0].InputMatchers = []protocol.InputMatcher{{Kind: "image"}}
			mf.Contributes.EntryPoints = []protocol.EntryPoint{{ID: "recognize-image", Version: "1", CommandID: "open", Input: "image"}}
		} else {
			mf.Dependencies = []protocol.Dependency{{ID: "ocr", PluginID: "example.target", MinVersion: "0.3.0", Optional: true, Entries: []protocol.EntryRequirement{{ID: "recognize-image", Version: "1"}}}}
			mf.Contributes.UIViews[0].Dependencies = []string{"ocr"}
		}
		raw, _ := json.Marshal(mf)
		os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
		if _, err := m.Install(dir, false); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	targetDir := install("example.target", true)
	install("example.source", false)
	owner, err := m.OpenUIView(context.Background(), "example.source", "main")
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.OpenUIView(context.Background(), "example.source", "main")
	if err != nil {
		t.Fatal(err)
	}
	window, otherWindow := &application.WebviewWindow{}, &application.WebviewWindow{}
	source := &embeddedEntry{window: window, session: owner}
	h := &HostService{manager: m, launcher: &launcher{window: window}, embedded: &embeddedViews{entries: map[*application.WebviewWindow]*embeddedEntry{window: source, otherWindow: {window: otherWindow, session: other}}, sequence: map[*application.WebviewWindow]uint64{window: 1}}}
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 8, 4)))
	value, _ := json.Marshal(map[string]any{"entryId": "recognize-image", "size": pngData.Len()})
	result, err := h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "image.upload.start", Input: "ocr", Value: value})
	if err != nil {
		t.Fatal(err)
	}
	var upload struct {
		ID string `json:"id"`
	}
	json.Unmarshal(result, &upload)
	if _, err = h.callDependencyHost(context.Background(), other, protocol.UIRequest{Method: "image.upload.finish", Input: upload.ID}); err == nil {
		t.Fatal("another source window stole upload")
	}
	if _, err = h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "image.upload.chunk", Input: upload.ID + ":0:" + base64.StdEncoding.EncodeToString(pngData.Bytes())}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "image.upload.finish", Input: upload.ID}); err != nil {
		t.Fatal(err)
	}
	mf, _ := manifest.Load(targetDir)
	mf.Version = "0.4.0"
	raw, _ := json.Marshal(mf)
	os.WriteFile(filepath.Join(targetDir, "manifest.json"), raw, 0600)
	if _, err = m.Upgrade(targetDir, false); err != nil {
		t.Fatal(err)
	}
	open, _ := json.Marshal(map[string]string{"entryId": "recognize-image", "imageHandle": upload.ID})
	if _, err = h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "dependency.open", Input: "ocr", Value: open}); err == nil {
		t.Fatal("replaced target received old handle")
	}
	if !owner.Active() || h.embedded.entries[window] != source {
		t.Fatal("failed handoff lost source")
	}
	source.imageUpload.close()
	if _, err = h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "image.upload.start", Input: "ocr", Value: value}); err != nil {
		t.Fatal(err)
	}
	source.imageUpload.close()
	if _, err = h.callDependencyHost(context.Background(), owner, protocol.UIRequest{Method: "image.upload.start", Input: "ocr", Value: value}); err == nil {
		t.Fatal("upload quota ignored")
	}
}
