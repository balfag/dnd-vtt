package httpapi

import (
	"errors"
	"net/http"

	"github.com/balfag/dnd-vtt/internal/game"
)

type gameSessionJSON struct {
	ID        string `json:"id"`
	StartedAt int64  `json:"startedAt"`
}

type campaignJSON struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	CreatedAt         int64            `json:"createdAt"`
	ActiveGameSession *gameSessionJSON `json:"activeGameSession"` // null when no active game
}

type startedGameJSON struct {
	ID         string `json:"id"`
	CampaignID string `json:"campaignId"`
	StartedAt  int64  `json:"startedAt"`
	Link       string `json:"link"`
}

type playerJSON struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	AccountLogin string `json:"accountLogin"`
	DisplayName  string `json:"displayName"`
	Color        string `json:"color"`
	Role         string `json:"role"`
	Removed      bool   `json:"removed"`
	Online       bool   `json:"online"`
}

func newCampaignJSON(c game.Campaign) campaignJSON {
	out := campaignJSON{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt.UnixMilli()}
	if c.ActiveGameSession != nil {
		out.ActiveGameSession = &gameSessionJSON{
			ID:        c.ActiveGameSession.ID,
			StartedAt: c.ActiveGameSession.StartedAt.UnixMilli(),
		}
	}
	return out
}

// gameLinkURL builds the link players open: the scheme and host the GM used
// for this request, then /play/<token>. The external address (port forwarding
// or tunnel) is decided by spike #8 and will replace this.
func gameLinkURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/play/" + token
}

// gameError answers with the response matching a game.Service error.
func (a *api) gameError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, game.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "bad_request", "invalid name")
	case errors.Is(err, game.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, game.ErrCannotRemoveGM):
		writeError(w, http.StatusConflict, "conflict", "the gm cannot be removed")
	default:
		a.internalError(w, action, err)
	}
}

func (a *api) listCampaigns(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	campaigns, err := a.d.Game.ListCampaigns(r.Context())
	if err != nil {
		a.gameError(w, "list campaigns", err)
		return
	}
	// Never nil: an empty list must encode as [], not null.
	out := make([]campaignJSON, 0, len(campaigns))
	for _, c := range campaigns {
		out = append(out, newCampaignJSON(c))
	}
	writeJSON(w, http.StatusOK, out)
}

type nameRequest struct {
	Name string `json:"name"`
}

func (a *api) createCampaign(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req nameRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	c, err := a.d.Game.CreateCampaign(r.Context(), req.Name)
	if err != nil {
		a.gameError(w, "create campaign", err)
		return
	}
	writeJSON(w, http.StatusCreated, newCampaignJSON(c))
}

func (a *api) renameCampaign(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req nameRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	c, err := a.d.Game.RenameCampaign(r.Context(), r.PathValue("id"), req.Name)
	if err != nil {
		a.gameError(w, "rename campaign", err)
		return
	}
	writeJSON(w, http.StatusOK, newCampaignJSON(c))
}

func (a *api) startGameSession(w http.ResponseWriter, r *http.Request) {
	acc, _, ok := a.requireGM(w, r)
	if !ok {
		return
	}
	gs, token, err := a.d.Game.StartGameSession(r.Context(), r.PathValue("id"), acc)
	if err != nil {
		a.gameError(w, "start game session", err)
		return
	}
	a.d.Hub.GameStarted(r.Context())
	// The link carries a secret token, so it is not logged.
	a.d.Logger.Info("game session started", "campaignId", gs.CampaignID, "gameSessionId", gs.ID)
	writeJSON(w, http.StatusCreated, startedGameJSON{
		ID:         gs.ID,
		CampaignID: gs.CampaignID,
		StartedAt:  gs.StartedAt.UnixMilli(),
		Link:       gameLinkURL(r, token),
	})
}

func (a *api) endGameSession(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if err := a.d.Game.EndGameSession(r.Context(), id); err != nil {
		a.gameError(w, "end game session", err)
		return
	}
	a.d.Hub.GameEnded(r.Context())
	a.d.Logger.Info("game session ended", "gameSessionId", id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) rotateLink(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	token, err := a.d.Game.RotateLink(r.Context(), r.PathValue("id"))
	if err != nil {
		a.gameError(w, "rotate game link", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"link": gameLinkURL(r, token)})
}

func (a *api) listPlayers(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	players, err := a.d.Game.ListPlayers(r.Context(), r.PathValue("id"))
	if err != nil {
		a.gameError(w, "list players", err)
		return
	}
	online := a.d.Hub.Online()
	out := make([]playerJSON, 0, len(players))
	for _, p := range players {
		out = append(out, playerJSON{
			ID:           p.ID,
			AccountID:    p.AccountID,
			AccountLogin: p.AccountLogin,
			DisplayName:  p.DisplayName,
			Color:        p.Color,
			Role:         p.Role,
			Removed:      p.Removed,
			Online:       online[p.ID],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) removePlayer(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Game.RemovePlayer(r.Context(), r.PathValue("id")); err != nil {
		a.gameError(w, "remove player", err)
		return
	}
	a.d.Hub.PlayerRemoved(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}
