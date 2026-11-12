// Package icon reads bounded, passive plugin artwork for trusted host UI.
package icon

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxBytes = 64 * 1024

func ValidPath(path string) bool {
	if path == "" {
		return true
	}
	if len(path) > 256 || !filepath.IsLocal(path) || strings.ContainsAny(path, `\:$`) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg", ".png", ".jpg", ".jpeg":
		return true
	}
	return false
}

// Load never returns a filesystem path or reads outside the plugin root.
// Missing, malformed and unsupported artwork falls back to the plugin initial.
func Load(dir, path string) string {
	if path == "" || !ValidPath(path) {
		return ""
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer root.Close()
	f, err := root.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return ""
	}
	return DataURL(data)
}

func DataURL(data []byte) string {
	if len(data) == 0 || len(data) > MaxBytes {
		return ""
	}
	mime := "image/svg+xml"
	if !validSVG(data) {
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 1024 || config.Height > 1024 {
			return ""
		}
		switch format {
		case "png":
			mime = "image/png"
		case "jpeg":
			mime = "image/jpeg"
		default:
			return ""
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// Normalize accepts embedded catalog artwork only; no remote URLs or file access.
func Normalize(value string) string {
	if len(value) > MaxBytes*2 {
		return ""
	}
	for _, mime := range []string{"image/svg+xml", "image/png", "image/jpeg"} {
		if encoded, ok := strings.CutPrefix(value, "data:"+mime+";base64,"); ok {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return ""
			}
			return DataURL(data)
		}
	}
	return ""
}

func validSVG(data []byte) bool {
	elements := map[string]bool{"svg": true, "g": true, "path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "title": true, "desc": true}
	attrs := map[string]bool{"xmlns": true, "viewBox": true, "width": true, "height": true, "x": true, "y": true, "x1": true, "x2": true, "y1": true, "y2": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true, "d": true, "points": true, "fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-opacity": true, "stroke-dasharray": true, "stroke-dashoffset": true, "opacity": true, "transform": true, "clip-rule": true}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots, count := 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return roots == 1 && depth == 0
		}
		if err != nil {
			return false
		}
		switch t := token.(type) {
		case xml.StartElement:
			count++
			if count > 512 || depth > 32 || !elements[t.Name.Local] || t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				return false
			}
			if depth == 0 {
				roots++
				if t.Name.Local != "svg" || roots != 1 {
					return false
				}
			}
			depth++
			for _, a := range t.Attr {
				if !attrs[a.Name.Local] || a.Name.Space != "" || strings.ContainsAny(a.Value, `\&`) || strings.Contains(strings.ToLower(a.Value), "url") {
					return false
				}
				if a.Name.Local == "xmlns" && a.Value != "http://www.w3.org/2000/svg" {
					return false
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive, xml.ProcInst:
			return false
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return false
			}
		}
	}
}
