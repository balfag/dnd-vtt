// Package httpapi contains the HTTP routes of the application.
package httpapi

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// NewRouter returns the handler for all HTTP routes of the application.
// spa is the built web client (see web.Dist).
func NewRouter(logger *slog.Logger, version string, spa fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: version}); err != nil {
			logger.Error("write health response", "err", err)
		}
	})
	// Unknown API paths must be 404, not the SPA fallback. The method in the
	// pattern avoids a conflict with "GET /".
	mux.Handle("GET /api/", http.NotFoundHandler())
	mux.Handle("GET /", spaHandler(spa))
	return mux
}
