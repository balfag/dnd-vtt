package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withLocalPort puts the local address of the connection into the context,
// as net/http does for a real request.
func withLocalPort(port int) func(*http.Request) {
	return func(r *http.Request) {
		addr := &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: port}
		*r = *r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, addr))
	}
}

func TestRequireHTTPS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	tests := []struct {
		name     string
		method   string
		path     string
		remote   string
		tls      bool
		wantCode int
		wantLoc  string
	}{
		{name: "loopback http", method: "GET", path: "/x", remote: "127.0.0.1:5000", wantCode: 204},
		{name: "lan get redirects", method: "GET", path: "/x?y=1", remote: "192.168.1.10:5000", wantCode: 308, wantLoc: "https://example.com/x?y=1"},
		{name: "lan head redirects", method: "HEAD", path: "/x", remote: "192.168.1.10:5000", wantCode: 308, wantLoc: "https://example.com/x"},
		{name: "lan post", method: "POST", path: "/x", remote: "192.168.1.10:5000", wantCode: 403},
		{name: "lan websocket", method: "GET", path: "/ws", remote: "192.168.1.10:5000", wantCode: 403},
		{name: "lan tls", method: "POST", path: "/x", remote: "192.168.1.10:5000", tls: true, wantCode: 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = tt.remote
			if tt.tls {
				withTLS(req)
			}
			rec := httptest.NewRecorder()
			requireHTTPS(next).ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLoc {
				t.Errorf("Location = %q, want %q", got, tt.wantLoc)
			}
			if tt.wantCode == 403 && errorCode(t, rec) != "https_required" {
				t.Errorf("code = %q, want https_required", errorCode(t, rec))
			}
		})
	}
}

// TestRouterRequireHTTPSOnlyWithTLS checks the wiring: the middleware is in
// the chain only when Deps.TLS is set.
func TestRouterRequireHTTPSOnlyWithTLS(t *testing.T) {
	for _, tlsOn := range []bool{true, false} {
		f := netFixture(t, tlsOn, nil)
		rec := f.do(http.MethodPost, "/api/auth/logout", "", remote)
		blocked := rec.Code == http.StatusForbidden && errorCode(t, rec) == "https_required"
		if blocked != tlsOn {
			t.Errorf("TLS=%v: status = %d, blocked = %v", tlsOn, rec.Code, blocked)
		}
	}
}

// netFixture is an apiFixture whose router has the LAN settings of the test.
func netFixture(t *testing.T, tlsOn bool, hosts func() ([]string, error)) *apiFixture {
	t.Helper()
	f := newAPIFixture(t)
	d := f.deps
	d.TLS = tlsOn
	d.LANHosts = hosts
	f.h = NewRouter(d)
	return f
}

func TestNetAddresses(t *testing.T) {
	hosts := func() ([]string, error) { return []string{"192.168.1.5", "10.0.0.3"}, nil }
	f := netFixture(t, true, hosts)

	gm := f.gmCookie()
	_, link := f.startGame(gm, f.createCampaign(gm, "Camp"))
	player, _ := f.playerSession(link, "bob")
	if rec := f.do(http.MethodGet, "/api/net/addresses", "", withCookie(player), withLocalPort(7777)); rec.Code != http.StatusForbidden {
		t.Errorf("player status = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/net/addresses", ""); rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("anonymous status = %d, want 401 or 403", rec.Code)
	}

	rec := f.do(http.MethodGet, "/api/net/addresses", "", withCookie(gm), withLocalPort(7777))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got netAddressesResponse
	decodeBody(t, rec, &got)
	if got.Scheme != "https" || got.Port != 7777 || len(got.Hosts) != 2 || got.Hosts[0] != "192.168.1.5" {
		t.Errorf("response = %+v", got)
	}
}

func TestNetAddressesEmptyAndHTTP(t *testing.T) {
	f := netFixture(t, false, nil)
	gm := f.gmCookie()
	rec := f.do(http.MethodGet, "/api/net/addresses", "", withCookie(gm), withLocalPort(8080))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); body != "{\"scheme\":\"http\",\"port\":8080,\"hosts\":[]}\n" {
		t.Errorf("body = %q", body)
	}
}

func TestNetAddressesError(t *testing.T) {
	f := netFixture(t, true, func() ([]string, error) { return nil, errors.New("boom") })
	gm := f.gmCookie()
	rec := f.do(http.MethodGet, "/api/net/addresses", "", withCookie(gm), withLocalPort(7777))
	if rec.Code != http.StatusInternalServerError || errorCode(t, rec) != "internal" {
		t.Errorf("status = %d, want 500 internal", rec.Code)
	}
}
