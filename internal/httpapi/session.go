package httpapi

import (
	"errors"
	"net/http"

	"github.com/balfag/dnd-vtt/internal/auth"
)

const sessionCookieName = "dndvtt_session"

// setSessionCookie: Path "/", HttpOnly, SameSite=Lax, Secure only if r.TLS != nil,
// Expires = sess.ExpiresAt.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, sess auth.Session) {
	// Secure only over HTTPS: players on a LAN use plain http://, and a Secure
	// cookie would never be stored there (see docs/architecture/security.md).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure depends on TLS on purpose
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}

// clearSessionCookie: same attributes, empty value, MaxAge -1.
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	// Secure only over HTTPS: players on a LAN use plain http://, and a Secure
	// cookie would never be stored there (see docs/architecture/security.md).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure depends on TLS on purpose
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}

// requireSession resolves the cookie with Auth.ResolveSession. On a missing
// cookie or auth.ErrNoSession it writes 401 unauthorized and returns ok=false.
func (a *api) requireSession(w http.ResponseWriter, r *http.Request) (auth.Account, auth.Session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return auth.Account{}, auth.Session{}, false
	}
	acc, sess, err := a.d.Auth.ResolveSession(r.Context(), cookie.Value)
	if errors.Is(err, auth.ErrNoSession) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in required")
		return auth.Account{}, auth.Session{}, false
	}
	if err != nil {
		a.internalError(w, "resolve session", err)
		return auth.Account{}, auth.Session{}, false
	}
	return acc, sess, true
}

// internalError logs err and answers 500 without leaking details.
func (a *api) internalError(w http.ResponseWriter, action string, err error) {
	a.d.Logger.Error(action, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}
