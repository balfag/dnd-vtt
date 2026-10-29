package httpapi

import "net/http"

// Values of the security headers, exactly as in docs/architecture/security.md ("Заголовки").
const (
	headerCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; font-src 'self'; connect-src 'self' ws: wss:; " +
		"worker-src 'self' blob:; media-src 'self' blob:; object-src 'none'; " +
		"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	headerReferrerPolicy     = "no-referrer"
	headerContentTypeOptions = "nosniff"
	headerPermissionsPolicy  = "camera=(self), microphone=(self), display-capture=(self), geolocation=(), payment=(), usb=()"
	headerCOOP               = "same-origin"
)

// securityHeaders sets the security headers before next writes anything, so
// they reach every response. Handlers may override a value with Header().Set
// (serveAsset does this for the CSP).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", headerCSP)
		h.Set("Referrer-Policy", headerReferrerPolicy)
		h.Set("X-Content-Type-Options", headerContentTypeOptions)
		h.Set("Permissions-Policy", headerPermissionsPolicy)
		h.Set("Cross-Origin-Opener-Policy", headerCOOP)
		next.ServeHTTP(w, r)
	})
}
