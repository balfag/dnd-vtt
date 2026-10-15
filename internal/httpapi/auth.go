package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/balfag/dnd-vtt/internal/auth"
)

type meResponse struct {
	AccountID string `json:"accountId"`
	Login     string `json:"login"`
	IsGM      bool   `json:"isGm"`
	Player    any    `json:"player"` // always null until #16
}

func newMe(acc auth.Account) meResponse {
	return meResponse{AccountID: acc.ID, Login: acc.Login, IsGM: acc.IsGM}
}

// limitKey is one rate-limit bucket checked before an expensive operation.
type limitKey struct {
	key   string
	limit int
}

func ipKey(r *http.Request) limitKey {
	return limitKey{key: "ip:" + clientIP(r), limit: limitPerIP}
}

func loginKey(r *http.Request, login string) limitKey {
	login = strings.ToLower(strings.TrimSpace(login))
	return limitKey{key: "login:" + clientIP(r) + "|" + login, limit: limitPerLogin}
}

// rateLimited answers 429 and returns true if any of keys is over its limit.
func (a *api) rateLimited(w http.ResponseWriter, keys ...limitKey) bool {
	for _, k := range keys {
		if blocked, retry := a.limiter.blocked(k.key, k.limit); blocked {
			secs := max(int(math.Ceil(retry.Seconds())), 1)
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
			return true
		}
	}
	return false
}

func (a *api) fail(keys ...limitKey) {
	for _, k := range keys {
		a.limiter.fail(k.key)
	}
}

// startGMSession creates a GM login session, sets the cookie and logs the sign-in.
// It reports false after writing an error response.
func (a *api) startGMSession(w http.ResponseWriter, r *http.Request, acc auth.Account, via string) bool {
	token, sess, err := a.d.Auth.CreateSession(r.Context(), acc.ID, auth.GMSessionTTL)
	if err != nil {
		a.internalError(w, "create login session", err)
		return false
	}
	setSessionCookie(w, r, token, sess)
	a.d.Logger.Info("gm signed in", "ip", clientIP(r), "via", via)
	return true
}

func (a *api) getSetup(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeError(w, http.StatusForbidden, "forbidden", "setup is only available from this machine")
		return
	}
	exists, err := a.d.Auth.GMExists(r.Context())
	if err != nil {
		a.internalError(w, "check gm account", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needed": !exists})
}

func (a *api) postSetup(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeError(w, http.StatusForbidden, "forbidden", "setup is only available from this machine")
		return
	}
	if a.rateLimited(w, ipKey(r)) {
		return
	}
	var req struct {
		SetupToken string `json:"setupToken"`
		Login      string `json:"login"`
		Password   string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	acc, err := a.d.Auth.CreateGM(r.Context(), req.SetupToken, req.Login, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		a.fail(ipKey(r))
		writeError(w, http.StatusForbidden, "forbidden", "invalid setup token")
		return
	case errors.Is(err, auth.ErrGMExists):
		writeError(w, http.StatusConflict, "conflict", "gm account already exists")
		return
	case errors.Is(err, auth.ErrInvalidLogin), errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusBadRequest, "bad_request", "invalid login or password")
		return
	case err != nil:
		a.internalError(w, "create gm", err)
		return
	}

	if !a.startGMSession(w, r, acc, "setup") {
		return
	}
	writeJSON(w, http.StatusCreated, newMe(acc))
}

func (a *api) postGMToken(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeError(w, http.StatusForbidden, "forbidden", "gm token is only accepted from this machine")
		return
	}
	if a.rateLimited(w, ipKey(r)) {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	acc, err := a.d.Auth.RedeemGMToken(r.Context(), req.Token)
	if errors.Is(err, auth.ErrInvalidToken) {
		a.fail(ipKey(r))
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return
	}
	if err != nil {
		a.internalError(w, "redeem gm token", err)
		return
	}

	if !a.startGMSession(w, r, acc, "gm-token") {
		return
	}
	writeJSON(w, http.StatusOK, newMe(acc))
}

func (a *api) postLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
		GameLink string `json:"gameLink"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	// Game links arrive with the game sessions work (#16).
	if req.GameLink != "" {
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return
	}

	keys := []limitKey{ipKey(r), loginKey(r, req.Login)}
	// Check the limits before Authenticate: a blocked brute force must not
	// spend argon2 time or learn whether a guess was right.
	if a.rateLimited(w, keys...) {
		return
	}

	acc, err := a.d.Auth.Authenticate(r.Context(), req.Login, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		a.fail(keys...)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid login or password")
		return
	}
	if err != nil {
		a.internalError(w, "authenticate", err)
		return
	}
	if !acc.IsGM {
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return
	}

	if !a.startGMSession(w, r, acc, "password") {
		return
	}
	writeJSON(w, http.StatusOK, newMe(acc))
}

func (a *api) postLogout(w http.ResponseWriter, r *http.Request) {
	_, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	if err := a.d.Auth.DeleteSession(r.Context(), sess.ID); err != nil {
		a.internalError(w, "delete login session", err)
		return
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) getMe(w http.ResponseWriter, r *http.Request) {
	acc, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	// requireSession succeeded, so the cookie exists. Re-set it so the browser
	// cookie follows a session the service has just extended.
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		setSessionCookie(w, r, cookie.Value, sess)
	}
	writeJSON(w, http.StatusOK, newMe(acc))
}

func (a *api) postPassword(w http.ResponseWriter, r *http.Request) {
	acc, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	key := loginKey(r, acc.Login)
	if a.rateLimited(w, key) {
		return
	}
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	err := a.d.Auth.ChangePassword(r.Context(), acc.ID, req.Current, req.New, sess.ID)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		a.fail(key)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "current password is wrong")
		return
	case errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusBadRequest, "bad_request", "new password is invalid")
		return
	case err != nil:
		a.internalError(w, "change password", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
