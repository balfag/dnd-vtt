package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/balfag/dnd-vtt/internal/asset"
)

// Literal copies of the table in docs/architecture/security.md: the tests must
// fail if the constants drift from the spec.
var wantSecurityHeaders = map[string]string{
	"Content-Security-Policy":    "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self' ws: wss:; worker-src 'self' blob:; media-src 'self' blob:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
	"Referrer-Policy":            "no-referrer",
	"X-Content-Type-Options":     "nosniff",
	"Permissions-Policy":         "camera=(self), microphone=(self), display-capture=(self), geolocation=(), payment=(), usb=()",
	"Cross-Origin-Opener-Policy": "same-origin",
}

func checkSecurityHeaders(t *testing.T, h http.Header, overrides map[string]string) {
	t.Helper()
	for name, want := range wantSecurityHeaders {
		if o, ok := overrides[name]; ok {
			want = o
		}
		if got := h.Values(name); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %q, want exactly [%q]", name, got, want)
		}
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	spa := fstest.MapFS{
		"index.html":  {Data: []byte("<html>app</html>")},
		"popout.html": {Data: []byte("<html>popout</html>")},
	}
	f := newAPIFixture(t)
	h := NewRouter(newTestDeps(t, spa))

	tests := []struct {
		name, path string
		want       int
	}{
		{"spa", "/", http.StatusOK},
		{"popout", "/popout.html", http.StatusOK},
		{"health", "/api/health", http.StatusOK},
		{"unauthorized", "/api/auth/me", http.StatusUnauthorized},
		{"unknown api", "/api/x", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			checkSecurityHeaders(t, rec.Header(), nil)
		})
	}

	t.Run("forbidden origin", func(t *testing.T) {
		rec := f.do(http.MethodPost, "/api/auth/logout", "", withHeader("Origin", "http://evil.example"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		checkSecurityHeaders(t, rec.Header(), nil)
	})
}

func TestSecurityHeadersOnServeAsset(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	a := f.mustUpload("a.png", testPNG(t, 3, 2))

	rec := f.do(http.MethodGet, a.URL, "", withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	checkSecurityHeaders(t, rec.Header(), map[string]string{"Content-Security-Policy": "sandbox"})
}

func TestSecurityHeadersOnFailedWS(t *testing.T) {
	f := newAPIFixture(t)
	rec := f.do(http.MethodGet, "/ws", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	checkSecurityHeaders(t, rec.Header(), nil)

	// A logged-in request without the upgrade headers is rejected by the hub itself.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(f.gmCookie())
	f.h.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status = %d, want an error", rec.Code)
	}
	checkSecurityHeaders(t, rec.Header(), nil)
}
