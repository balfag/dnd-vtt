package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
)

const maxBodyBytes = 64 << 10

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status line is already sent, so an encoding error cannot be reported
	// to the client; the connection error surfaces on the next write anyway.
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes {"error":{"code":..., "message":...}}.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: apiError{Code: code, Message: message}})
}

// decodeJSON reads a JSON object into dst. It requires Content-Type
// application/json (parameters like charset allowed), limits the body to
// 64 KiB, rejects unknown fields and trailing data.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON object")
	}
	return nil
}

// clientIP returns the IP of the TCP peer (r.RemoteAddr without port).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isLoopback reports whether the request came from this machine and was not
// forwarded: the peer IP is loopback and there are no X-Forwarded-For or
// Forwarded headers (a local tunnel agent would otherwise look local).
func isLoopback(r *http.Request) bool {
	if len(r.Header.Values("X-Forwarded-For")) > 0 || len(r.Header.Values("Forwarded")) > 0 {
		return false
	}
	ip := net.ParseIP(clientIP(r))
	return ip != nil && ip.IsLoopback()
}
