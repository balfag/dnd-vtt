package httpapi

import (
	"net"
	"net/http"
)

// localPort returns the port of the local address of the connection that
// carried r. It is not r.Host: a proxy may hide the port there.
func localPort(r *http.Request) (int, bool) {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	if !ok || addr == nil {
		return 0, false
	}
	return addr.Port, true
}

// requireHTTPS lets only TLS requests and loopback requests through: plain
// HTTP from other addresses gets a redirect to HTTPS (GET, HEAD) or 403
// https_required (everything else, and WebSocket) (ADR-0015).
func requireHTTPS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil || isLoopback(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path != "/ws" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			// Same host as the request, only the scheme changes.
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect) //nolint:gosec // G710: the target is the request's own host
			return
		}
		writeError(w, http.StatusForbidden, "https_required", "https required")
	})
}

type netAddressesResponse struct {
	Scheme string   `json:"scheme"`
	Port   int      `json:"port"`
	Hosts  []string `json:"hosts"`
}

// getNetAddresses lists the LAN addresses players can use to reach the server.
func (a *api) getNetAddresses(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	hosts := []string{}
	if a.d.LANHosts != nil {
		h, err := a.d.LANHosts()
		if err != nil {
			a.internalError(w, "list lan addresses", err)
			return
		}
		if h != nil {
			hosts = h
		}
	}
	scheme := "http"
	if a.d.TLS {
		scheme = "https"
	}
	port, _ := localPort(r)
	writeJSON(w, http.StatusOK, netAddressesResponse{Scheme: scheme, Port: port, Hosts: hosts})
}
