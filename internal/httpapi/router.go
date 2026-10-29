// Package httpapi contains the HTTP routes of the application.
package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/vtt"
	"github.com/balfag/dnd-vtt/internal/wiki"
	"github.com/balfag/dnd-vtt/internal/ws"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Deps are the dependencies of the HTTP API.
type Deps struct {
	Logger  *slog.Logger
	Version string
	SPA     fs.FS // built web client, see web.Dist
	Auth    *auth.Service
	Game    *game.Service
	Hub     *ws.Hub
	Assets  *asset.Service
	VTT     *vtt.Service
	Wiki    *wiki.Service
	Now     func() time.Time // nil means time.Now; used by the rate limiter
}

// api holds the state shared by the handlers.
type api struct {
	d       Deps
	limiter *limiter
}

// NewRouter returns the handler for all HTTP routes of the application.
func NewRouter(d Deps) http.Handler {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	a := &api{d: d, limiter: newLimiter(failWindow, now)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: d.Version})
	})
	mux.HandleFunc("GET /api/setup", a.getSetup)
	mux.HandleFunc("POST /api/setup", a.postSetup)
	mux.HandleFunc("POST /api/auth/gm-token", a.postGMToken)
	mux.HandleFunc("POST /api/auth/login", a.postLogin)
	mux.HandleFunc("POST /api/auth/logout", a.postLogout)
	mux.HandleFunc("GET /api/auth/me", a.getMe)
	mux.HandleFunc("POST /api/auth/password", a.postPassword)
	mux.HandleFunc("POST /api/auth/register", a.postRegister)
	mux.HandleFunc("GET /api/game-links/{token}", a.getGameLink)
	mux.HandleFunc("POST /api/accounts/{id}/password-resets", a.createPasswordReset)
	mux.HandleFunc("POST /api/password-resets/{token}", a.applyPasswordReset)
	mux.HandleFunc("POST /api/campaigns/{id}/assets", a.uploadAsset)
	mux.HandleFunc("GET /api/campaigns/{id}/assets", a.listAssets)
	mux.HandleFunc("DELETE /api/assets/{id}", a.deleteAsset)
	mux.HandleFunc("GET /assets/{id}", a.serveAsset)
	mux.HandleFunc("GET /ws", a.getWS)
	mux.HandleFunc("GET /api/campaigns/{id}/wiki/tree", a.getWikiTree)
	mux.HandleFunc("POST /api/campaigns/{id}/wiki/pages", a.createWikiPage)
	mux.HandleFunc("GET /api/wiki/pages/{id}", a.getWikiPage)
	mux.HandleFunc("PUT /api/wiki/pages/{id}", a.saveWikiPage)
	mux.HandleFunc("POST /api/wiki/pages/{id}/move", a.moveWikiPage)
	mux.HandleFunc("DELETE /api/wiki/pages/{id}", a.deleteWikiPage)
	mux.HandleFunc("PUT /api/wiki/pages/{id}/tags", a.setWikiPageTags)
	mux.HandleFunc("GET /api/campaigns/{id}/wiki/tags", a.listWikiTags)
	mux.HandleFunc("PATCH /api/wiki/tags/{id}", a.renameWikiTag)
	mux.HandleFunc("DELETE /api/wiki/tags/{id}", a.deleteWikiTag)
	mux.HandleFunc("GET /api/campaigns/{id}/wiki/trash", a.getWikiTrash)
	mux.HandleFunc("DELETE /api/campaigns/{id}/wiki/trash", a.emptyWikiTrash)
	mux.HandleFunc("POST /api/wiki/trash/{id}/restore", a.restoreWikiTrash)
	mux.HandleFunc("DELETE /api/wiki/trash/{id}", a.purgeWikiTrash)

	mux.HandleFunc("GET /api/campaigns", a.listCampaigns)
	mux.HandleFunc("POST /api/campaigns", a.createCampaign)
	mux.HandleFunc("PATCH /api/campaigns/{id}", a.renameCampaign)
	mux.HandleFunc("POST /api/campaigns/{id}/game-sessions", a.startGameSession)
	mux.HandleFunc("POST /api/game-sessions/{id}/end", a.endGameSession)
	mux.HandleFunc("POST /api/game-sessions/{id}/link", a.rotateLink)
	mux.HandleFunc("GET /api/campaigns/{id}/players", a.listPlayers)
	mux.HandleFunc("POST /api/players/{id}/remove", a.removePlayer)
	// Unknown API paths must be 404, not the SPA fallback. The method in the
	// pattern avoids a conflict with "GET /".
	mux.Handle("GET /api/", http.NotFoundHandler())
	mux.Handle("GET /", spaHandler(d.SPA))
	return checkOrigin(mux)
}

// checkOrigin rejects state-changing requests whose Origin header does not
// match the request's own scheme and host (CSRF protection). Requests
// without Origin (curl, same-origin non-browser clients) pass.
func checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != scheme || !strings.EqualFold(u.Host, r.Host) {
			writeError(w, http.StatusForbidden, "forbidden", "cross-origin request")
			return
		}
		next.ServeHTTP(w, r)
	})
}
