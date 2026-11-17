package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"sinan/internal/paste"
	"sinan/internal/plugin/uiview"
	protocol "sinan/packages/plugin-protocol"
)

type dependencyImage struct {
	mu                            sync.Mutex
	id, alias, entry, fingerprint string
	data                          []byte
	content                       *paste.Content
	size                          int
	started, touched, expires     time.Time
	closed                        bool
}

func newDependencyImage(owner *uiview.Session, alias, entry, fingerprint string, size int) *dependencyImage {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil
	}
	now := time.Now()
	s := &dependencyImage{id: hex.EncodeToString(b[:]), alias: alias, entry: entry, fingerprint: fingerprint, size: size, started: now, touched: now}
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-owner.Done():
				s.close()
				return
			case now := <-tick.C:
				s.mu.Lock()
				expired := s.expired(now)
				s.mu.Unlock()
				if expired {
					s.close()
					return
				}
			}
		}
	}()
	return s
}
func (s *dependencyImage) expired(now time.Time) bool {
	return s.closed || (!s.expires.IsZero() && !now.Before(s.expires)) || (s.expires.IsZero() && (now.Sub(s.touched) > 15*time.Second || now.Sub(s.started) > 60*time.Second))
}
func (s *dependencyImage) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.data = nil
	s.content = nil
}
func (s *dependencyImage) append(input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := strings.SplitN(input, ":", 3)
	if s.expired(time.Now()) || s.content != nil || len(parts) != 3 || parts[0] != s.id {
		return errors.New("图片上传已失效")
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset != len(s.data) {
		return errors.New("图片分块顺序无效")
	}
	block, err := base64.StdEncoding.Strict().DecodeString(parts[2])
	remaining := s.size - len(s.data)
	if err != nil || len(block) != min(36<<10, remaining) || len(block) == 0 {
		return errors.New("图片分块大小无效")
	}
	s.data = append(s.data, block...)
	s.touched = time.Now()
	return nil
}
func (s *dependencyImage) finish(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != s.id || s.expired(time.Now()) || len(s.data) != s.size {
		return errors.New("图片尚未传输完成或已失效")
	}
	content, err := paste.Parse(protocol.PasteContent{Image: &protocol.PasteImage{DataBase64: base64.StdEncoding.EncodeToString(s.data)}})
	if err != nil {
		s.closed = true
		s.data = nil
		return err
	}
	s.content = content
	s.data = nil
	s.expires = time.Now().Add(30 * time.Second)
	return nil
}
func strictDependencyValue(raw json.RawMessage, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("依赖参数无效")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("依赖参数无效")
	}
	return nil
}

func (h *HostService) callDependencyHost(ctx context.Context, session *uiview.Session, req protocol.UIRequest) (json.RawMessage, error) {
	source, err := h.windowCommandEntry(ctx, session)
	if err != nil {
		return nil, err
	}
	defer h.finishWindowCommand(source, func() {})
	if req.Method == "dependency.manage" {
		// A fixed trusted destination; guest pages never choose a trusted component.
		if _, _, err := trustedView(h.manager.Snapshot(), "system.plugin-manager", "open"); err != nil {
			return nil, err
		}
		h.releaseEmbedded(source.window)
		h.launcher.mu.Lock()
		if detached, ok := h.launcher.detached[source.window.ID()]; ok {
			detached.key = "system.plugin-manager/manager"
			h.launcher.detached[source.window.ID()] = detached
		}
		h.launcher.mu.Unlock()
		source.window.DispatchWailsEvent(&application.CustomEvent{Name: "plugin-center:open", Sender: "kernel.dependencies"})
		h.launcher.reveal(source.window)
		source.window.Focus()
		return json.RawMessage(`null`), nil
	}
	switch req.Method {
	case "image.upload.start":
		var value struct {
			EntryID string `json:"entryId"`
			Size    int    `json:"size"`
		}
		if err = strictDependencyValue(req.Value, &value); err != nil {
			return nil, err
		}
		if value.Size < 1 || value.Size > paste.MaxImageBytes {
			return nil, errors.New("OCR 图片须小于 4 MiB，请先裁剪图片")
		}
		target, e := h.manager.ResolveDependency(session, req.Input, value.EntryID)
		if e != nil {
			return nil, e
		}
		now := time.Now()
		h.embedded.mu.Lock()
		defer h.embedded.mu.Unlock()
		if source.imageUpload != nil {
			source.imageUpload.mu.Lock()
			live := !source.imageUpload.expired(now)
			source.imageUpload.mu.Unlock()
			if live {
				return nil, errors.New("已有一份图片正在交接")
			}
		}
		if now.Sub(source.imageUploadWindow) >= time.Minute {
			source.imageUploadWindow = now
			source.imageUploads = 0
		}
		if source.imageUploads >= 2 {
			return nil, errors.New("图片交接过于频繁，请稍后重试")
		}
		source.imageUploads++
		source.imageUpload = newDependencyImage(session, req.Input, value.EntryID, target.Fingerprint, value.Size)
		if source.imageUpload == nil {
			return nil, errors.New("无法创建图片上传")
		}
		return json.Marshal(map[string]string{"id": source.imageUpload.id})
	case "image.upload.chunk", "image.upload.finish", "image.upload.cancel", "dependency.open":
		h.embedded.mu.Lock()
		upload := source.imageUpload
		h.embedded.mu.Unlock()
		if upload == nil {
			return nil, errors.New("图片上传不存在")
		}
		switch req.Method {
		case "image.upload.chunk":
			return json.RawMessage(`null`), upload.append(req.Input)
		case "image.upload.finish":
			if err = upload.finish(req.Input); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"imageHandle": upload.id})
		case "image.upload.cancel":
			if req.Input != upload.id {
				return nil, errors.New("图片上传标识无效")
			}
			upload.close()
			return json.RawMessage(`null`), nil
		}
		var value struct {
			EntryID     string `json:"entryId"`
			ImageHandle string `json:"imageHandle"`
		}
		if err = strictDependencyValue(req.Value, &value); err != nil {
			return nil, err
		}
		upload.mu.Lock()
		valid := !upload.expired(time.Now()) && upload.id == value.ImageHandle && upload.alias == req.Input && upload.entry == value.EntryID && upload.content != nil
		content, fingerprint := upload.content, upload.fingerprint
		upload.mu.Unlock()
		if !valid {
			return nil, errors.New("图片交接句柄无效或已消费")
		}
		check := func() bool {
			h.embedded.mu.Lock()
			defer h.embedded.mu.Unlock()
			return h.embedded.entries[source.window] == source && session.Active()
		}
		ctx = context.WithValue(ctx, application.WindowKey, source.window)
		err = h.openEmbeddedPrepared(ctx, "", "", "", nil, content, check, &handoffOpen{source: session, alias: req.Input, entry: value.EntryID, fingerprint: fingerprint})
		if err != nil {
			return nil, err
		}
		upload.close()
		return json.RawMessage(`null`), nil
	}
	return nil, errors.New("依赖方法不受支持")
}

type handoffOpen struct {
	source                    *uiview.Session
	alias, entry, fingerprint string
}
