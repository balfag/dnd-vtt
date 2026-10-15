package httpapi

import (
	"errors"
	"net/http"

	"github.com/balfag/dnd-vtt/internal/auth"
)

// createPasswordReset issues a one-time reset link for a player. The link
// carries a secret token, so it is not logged.
func (a *api) createPasswordReset(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	accountID := r.PathValue("id")
	token, expiresAt, err := a.d.Auth.CreatePasswordReset(r.Context(), accountID)
	switch {
	case errors.Is(err, auth.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	case errors.Is(err, auth.ErrCannotResetGM):
		writeError(w, http.StatusForbidden, "forbidden", "the gm password cannot be reset with a link")
		return
	case err != nil:
		a.internalError(w, "create password reset", err)
		return
	}
	a.d.Logger.Info("password reset issued", "accountId", accountID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"link":      publicURL(r, "/reset/"+token),
		"expiresAt": expiresAt.UnixMilli(),
	})
}

// applyPasswordReset sets a new password by the one-time token and closes the
// account's live connections. It needs no login: the token is the credential.
func (a *api) applyPasswordReset(w http.ResponseWriter, r *http.Request) {
	if a.rateLimited(w, ipKey(r)) {
		return
	}
	var req struct {
		New string `json:"new"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	accountID, err := a.d.Auth.ApplyPasswordReset(r.Context(), r.PathValue("token"), req.New)
	switch {
	case errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusBadRequest, "bad_request", "new password is invalid")
		return
	case errors.Is(err, auth.ErrInvalidToken):
		a.fail(ipKey(r))
		writeError(w, http.StatusForbidden, "forbidden", "invalid or expired reset link")
		return
	case err != nil:
		a.internalError(w, "apply password reset", err)
		return
	}
	a.d.Hub.CloseAccountSessions(accountID, "")
	a.d.Logger.Info("password reset applied", "accountId", accountID, "ip", clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}
