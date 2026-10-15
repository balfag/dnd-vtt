// Package httpapi contains the HTTP routes of the application.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// NewRouter returns the handler for all HTTP routes of the application.
func NewRouter(logger *slog.Logger, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: version}); err != nil {
			logger.Error("write health response", "err", err)
		}
	})
	return mux
}
