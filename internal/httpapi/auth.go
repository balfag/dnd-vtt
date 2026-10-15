package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
)

type mePlayerJSON struct {
	ID          string `json:"id"`
	CampaignID  string `json:"campaignId"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color"`
	Role        string `json:"role"`
}

type meResponse struct {
	AccountID string        `json:"accountId"`
	Login     string        `json:"login"`
	IsGM      bool          `json:"isGm"`
	Player    *mePlayerJSON `json:"player"` // null when the account has no participant in the active game
}

// me builds the Me response: the account plus its participant in the
// campaign of the active game session, if any.
func (a *api) me(ctx context.Context, acc auth.Account) (meResponse, error) {
	resp := meResponse{AccountID: acc.ID, Login: acc.Login, IsGM: acc.IsGM}

	gs, err := a.d.Game.ActiveGameSession(ctx)
	if errors.Is(err, game.ErrNoActiveGame) {
		return resp, nil
	}
	if err != nil {
		return meResponse{}, err
	}
	p, err := a.d.Game.PlayerForAccount(ctx, gs.CampaignID, acc.ID)
	if errors.Is(err, game.ErrNotFound) {
		return resp, nil
	}
	if err != nil {
		return meResponse{}, err
	}
	resp.Player = &mePlayerJSON{
		ID:          p.ID,
		CampaignID:  p.CampaignID,
		DisplayName: p.DisplayName,
		Color:       p.Color,
		Role:        p.Role,
	}
	return resp, nil
}

// writeMe answers with the Me response of acc, or 500 if it cannot be built.
func (a *api) writeMe(w http.ResponseWriter, r *http.Request, status int, acc auth.Account) {
	resp, err := a.me(r.Context(), acc)
	if err != nil {
		a.internalError(w, "build me response", err)
		return
	}
	writeJSON(w, status, resp)
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
	a.writeMe(w, r, http.StatusCreated, acc)
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
	a.writeMe(w, r, http.StatusOK, acc)
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

	keys := []limitKey{ipKey(r), loginKey(r, req.Login)}
	// Check the limits before Authenticate: a blocked brute force must not
	// spend argon2 time or learn whether a guess was right.
	if a.rateLimited(w, keys...) {
		return
	}

	// An invalid link is rejected before the password is checked, so it
	// cannot be used to guess passwords.
	var gs game.GameSession
	if req.GameLink != "" {
		var ok bool
		if gs, ok = a.resolveLink(w, r, req.GameLink); !ok {
			return
		}
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
	if acc.IsGM {
		if !a.startGMSession(w, r, acc, "password") {
			return
		}
		a.writeMe(w, r, http.StatusOK, acc)
		return
	}
	if req.GameLink == "" {
		// Players sign in only through a game link.
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return
	}

	p, err := a.d.Game.JoinPlayer(r.Context(), gs, acc)
	switch {
	case errors.Is(err, game.ErrRemoved):
		writeError(w, http.StatusForbidden, "removed", "you were removed from this campaign")
		return
	case errors.Is(err, game.ErrNoActiveGame):
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return
	case err != nil:
		a.internalError(w, "join player", err)
		return
	}
	if !a.startPlayerSession(w, r, acc, gs) {
		return
	}
	a.d.Logger.Info("player signed in", "ip", clientIP(r), "campaignId", p.CampaignID)
	a.writeMe(w, r, http.StatusOK, acc)
}

// resolveLink finds the active game session for a link token. An unknown or
// ended link counts as a failed attempt for the IP and answers 403.
func (a *api) resolveLink(w http.ResponseWriter, r *http.Request, token string) (game.GameSession, bool) {
	gs, err := a.d.Game.ResolveLink(r.Context(), token)
	if errors.Is(err, game.ErrNoActiveGame) {
		a.fail(ipKey(r))
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return game.GameSession{}, false
	}
	if err != nil {
		a.internalError(w, "resolve game link", err)
		return game.GameSession{}, false
	}
	return gs, true
}

// startPlayerSession creates a player login session bound to gs and sets the
// cookie. It reports false after writing an error response.
func (a *api) startPlayerSession(w http.ResponseWriter, r *http.Request, acc auth.Account, gs game.GameSession) bool {
	token, sess, err := a.d.Auth.CreatePlayerSession(r.Context(), acc.ID, gs.ID)
	if err != nil {
		a.internalError(w, "create player login session", err)
		return false
	}
	setSessionCookie(w, r, token, sess)
	return true
}

func (a *api) getGameLink(w http.ResponseWriter, r *http.Request) {
	if a.rateLimited(w, ipKey(r)) {
		return
	}
	gs, ok := a.resolveLink(w, r, r.PathValue("token"))
	if !ok {
		return
	}
	c, err := a.d.Game.GetCampaign(r.Context(), gs.CampaignID)
	if err != nil {
		a.internalError(w, "get campaign", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"campaignName": c.Name})
}

func (a *api) postRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GameLink    string `json:"gameLink"`
		Login       string `json:"login"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	if a.rateLimited(w, ipKey(r)) {
		return
	}
	gs, ok := a.resolveLink(w, r, req.GameLink)
	if !ok {
		return
	}

	acc, p, err := a.d.Game.RegisterPlayer(r.Context(), gs, req.Login, req.Password, req.DisplayName)
	switch {
	case errors.Is(err, auth.ErrInvalidLogin), errors.Is(err, auth.ErrInvalidPassword), errors.Is(err, game.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "bad_request", "invalid login, password or display name")
		return
	case errors.Is(err, game.ErrLoginTaken):
		writeError(w, http.StatusConflict, "conflict", "login is already taken")
		return
	case errors.Is(err, game.ErrNoActiveGame):
		writeError(w, http.StatusForbidden, "no_active_game", "there is no active game")
		return
	case err != nil:
		a.internalError(w, "register player", err)
		return
	}
	if !a.startPlayerSession(w, r, acc, gs) {
		return
	}
	a.d.Logger.Info("player registered", "ip", clientIP(r), "campaignId", p.CampaignID)
	a.writeMe(w, r, http.StatusCreated, acc)
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
	a.writeMe(w, r, http.StatusOK, acc)
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
