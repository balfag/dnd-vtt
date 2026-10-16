package asset

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func mustBase64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDetectType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string // "" means not allowed
	}{
		{"png", pngBytes(t, 2, 2), "image/png"},
		{"jpeg", jpegBytes(t, 2, 2), "image/jpeg"},
		{"gif", gifBytes(t, 2, 2), "image/gif"},
		{"webp", mustBase64(t, webp1x1), "image/webp"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), ""},
		{"text", []byte("hello"), ""},
		{"pdf", []byte("%PDF-1.4"), ""},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := detectType(tt.data)
			if tt.want == "" {
				if ok {
					t.Fatalf("detectType = %q, want not allowed", got.mime)
				}
				return
			}
			if !ok || got.mime != tt.want {
				t.Fatalf("detectType = %q, %v; want %q", got.mime, ok, tt.want)
			}
		})
	}
}

func TestImageSize(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		mime          string
		width, height int
	}{
		{"png", pngBytes(t, 7, 3), "image/png", 7, 3},
		{"jpeg", jpegBytes(t, 9, 5), "image/jpeg", 9, 5},
		{"gif", gifBytes(t, 4, 6), "image/gif", 4, 6},
		{"webp", mustBase64(t, webp1x1), "image/webp", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, _ := typeByMIME(tt.mime)
			w, h, ok := imageSize(bytes.NewReader(tt.data), typ)
			if !ok || w != tt.width || h != tt.height {
				t.Fatalf("imageSize = %d x %d, %v; want %d x %d", w, h, ok, tt.width, tt.height)
			}
		})
	}
}

func TestImageSizeStartsFromTheBeginning(t *testing.T) {
	r := bytes.NewReader(pngBytes(t, 7, 3))
	if _, err := r.Seek(20, 0); err != nil {
		t.Fatal(err)
	}
	typ, _ := typeByMIME("image/png")
	if w, h, ok := imageSize(r, typ); !ok || w != 7 || h != 3 {
		t.Fatalf("imageSize = %d x %d, %v; want 7 x 3", w, h, ok)
	}
}

func TestImageSizeRejects(t *testing.T) {
	png, _ := typeByMIME("image/png")
	jpg, _ := typeByMIME("image/jpeg")
	if _, _, ok := imageSize(bytes.NewReader(pngBytes(t, 2, 2)), jpg); ok {
		t.Error("PNG data accepted as JPEG: the detected type and the format must match")
	}
	if _, _, ok := imageSize(bytes.NewReader([]byte("\x89PNG\r\n\x1a\n\x00")), png); ok {
		t.Error("a cut PNG header was accepted")
	}
}
