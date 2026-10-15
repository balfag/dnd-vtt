package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const immutable = "public, max-age=31536000, immutable"

func TestSPA(t *testing.T) {
	built := fstest.MapFS{
		"index.html":      {Data: []byte("<html>app</html>")},
		"static/app-1.js": {Data: []byte("js")},
		".gitkeep":        {},
		"static/.hidden":  {Data: []byte("x")},
	}
	unbuilt := fstest.MapFS{".gitkeep": {}}

	tests := []struct {
		name       string
		fsys       fstest.MapFS
		method     string
		path       string
		wantCode   int
		wantCache  string
		wantBody   string // exact; empty means not checked
		wantInBody string // substring
	}{
		{"root", built, "GET", "/", 200, "no-cache", "<html>app</html>", ""},
		{"client route", built, "GET", "/campaign/42", 200, "no-cache", "<html>app</html>", ""},
		{"head client route", built, "HEAD", "/campaign/42", 200, "no-cache", "", ""},
		{"static asset", built, "GET", "/static/app-1.js", 200, immutable, "js", ""},
		{"missing static", built, "GET", "/static/missing.js", 404, "", "", ""},
		{"missing file with ext", built, "GET", "/favicon.ico", 404, "", "", ""},
		{"dotfile", built, "GET", "/.gitkeep", 404, "", "", ""},
		{"hidden static", built, "GET", "/static/.hidden", 404, "", "", ""},
		{"directory", built, "GET", "/static", 200, "no-cache", "<html>app</html>", ""},
		{"unknown api", built, "GET", "/api/unknown", 404, "", "", ""},
		{"post root", built, "POST", "/", 405, "", "", ""},
		{"health", built, "GET", "/api/health", 200, "", "", ""},
		{"unbuilt root", unbuilt, "GET", "/", 503, "", "", "pnpm -C web build"},
		{"unbuilt route", unbuilt, "GET", "/join/abc", 503, "", "", "pnpm -C web build"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), "test", tt.fsys)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantCache != "" && rec.Header().Get("Cache-Control") != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.wantCache)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if tt.method == http.MethodHead && rec.Body.Len() != 0 {
				t.Errorf("HEAD body = %q, want empty", rec.Body.String())
			}
			if tt.wantInBody != "" && !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantInBody)
			}
		})
	}
}

func TestSPAStaticContentType(t *testing.T) {
	fsys := fstest.MapFS{"static/app-1.js": {Data: []byte("js")}}
	h := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), "test", fsys)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/static/app-1.js", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", ct)
	}
}
