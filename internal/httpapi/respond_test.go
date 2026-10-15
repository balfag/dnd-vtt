package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	type body struct {
		A string `json:"a"`
	}
	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     bool
	}{
		{"valid", "application/json", `{"a":"x"}`, false},
		{"valid with charset", "application/json; charset=utf-8", `{"a":"x"}`, false},
		{"unknown field", "application/json", `{"a":"x","b":1}`, true},
		{"wrong content type", "text/plain", `{"a":"x"}`, true},
		{"missing content type", "", `{"a":"x"}`, true},
		{"too large", "application/json", `{"a":"` + strings.Repeat("x", 64<<10) + `"}`, true},
		{"two objects", "application/json", `{"a":"x"}{"a":"y"}`, true},
		{"empty body", "application/json", ``, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			var dst body
			err := decodeJSON(httptest.NewRecorder(), req, &dst)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && dst.A != "x" {
				t.Errorf("dst = %+v, want A=x", dst)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusTeapot, "x", "y")

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"error":{"code":"x","message":"y"}}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		header     string
		value      string
		want       bool
	}{
		{"ipv4 loopback", "127.0.0.1:1", "", "", true},
		{"ipv6 loopback", "[::1]:1", "", "", true},
		{"remote", "192.0.2.1:1", "", "", false},
		{"forwarded-for", "127.0.0.1:1", "X-Forwarded-For", "203.0.113.5", false},
		{"forwarded", "127.0.0.1:1", "Forwarded", "for=203.0.113.5", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}
			if got := isLoopback(req); got != tt.want {
				t.Errorf("isLoopback = %v, want %v", got, tt.want)
			}
		})
	}
}
