package httpapi

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const playerPassword = "player-password"

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("body %q is not the expected JSON: %v", rec.Body.String(), err)
	}
}

func withTLS(r *http.Request) { r.TLS = &tls.ConnectionState{} }

// tokenFromLink returns the part of a game link after /play/.
func tokenFromLink(link string) string {
	_, token, _ := strings.Cut(link, "/play/")
	return token
}

// gmCookie creates the GM account and returns its session cookie.
func (f *apiFixture) gmCookie() *http.Cookie {
	f.t.Helper()
	return sessionCookie(f.t, f.createGM())
}

func (f *apiFixture) createCampaign(cookie *http.Cookie, name string) string {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/campaigns", jsonBody(f.t, map[string]string{"name": name}), withCookie(cookie))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("create campaign = %d %s", rec.Code, rec.Body.String())
	}
	var c campaignJSON
	decodeBody(f.t, rec, &c)
	return c.ID
}

func (f *apiFixture) startGame(cookie *http.Cookie, campaignID string) (gameSessionID, link string) {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/campaigns/"+campaignID+"/game-sessions", "", withCookie(cookie))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("start game = %d %s", rec.Code, rec.Body.String())
	}
	var g startedGameJSON
	decodeBody(f.t, rec, &g)
	return g.ID, g.Link
}

// registerPlayer registers through the game link with the shared test password.
func (f *apiFixture) registerPlayer(link, login string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(http.MethodPost, "/api/auth/register", jsonBody(f.t, map[string]string{
		"gameLink": tokenFromLink(link), "login": login, "password": playerPassword, "displayName": "Player " + login,
	}))
}

// playerSession registers a player and returns the cookie and the Me response.
func (f *apiFixture) playerSession(link, login string) (*http.Cookie, meResponse) {
	f.t.Helper()
	rec := f.registerPlayer(link, login)
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	var me meResponse
	decodeBody(f.t, rec, &me)
	return sessionCookie(f.t, rec), me
}

func TestGMOnlyRoutes(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")
	gsID, link := f.startGame(gm, campaignID)
	player, me := f.playerSession(link, "bob")

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/campaigns", ""},
		{http.MethodPost, "/api/campaigns", `{"name":"x"}`},
		{http.MethodPatch, "/api/campaigns/" + campaignID, `{"name":"x"}`},
		{http.MethodPost, "/api/campaigns/" + campaignID + "/game-sessions", ""},
		{http.MethodPost, "/api/game-sessions/" + gsID + "/end", ""},
		{http.MethodPost, "/api/game-sessions/" + gsID + "/link", ""},
		{http.MethodGet, "/api/campaigns/" + campaignID + "/players", ""},
		{http.MethodPost, "/api/players/" + me.Player.ID + "/remove", ""},
	}
	for _, rt := range routes {
		name := rt.method + " " + rt.path
		if rec := f.do(rt.method, rt.path, rt.body); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthorized" {
			t.Errorf("%s without cookie = %d, want 401 unauthorized", name, rec.Code)
		}
		if rec := f.do(rt.method, rt.path, rt.body, withCookie(player)); rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
			t.Errorf("%s as a player = %d, want 403 forbidden", name, rec.Code)
		}
	}
}

func TestCampaignsAPI(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()

	rec := f.do(http.MethodGet, "/api/campaigns", "", withCookie(gm))
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("empty list = %d %q, want 200 []", rec.Code, rec.Body.String())
	}

	id := f.createCampaign(gm, "Первая")
	rec = f.do(http.MethodGet, "/api/campaigns", "", withCookie(gm))
	var list []campaignJSON
	decodeBody(t, rec, &list)
	if len(list) != 1 || list[0].ID != id || list[0].Name != "Первая" || list[0].ActiveGameSession != nil {
		t.Errorf("list = %+v", list)
	}
	if !strings.Contains(rec.Body.String(), `"activeGameSession":null`) {
		t.Errorf("body %s must contain activeGameSession:null", rec.Body.String())
	}

	if rec := f.do(http.MethodPost, "/api/campaigns", `{"name":"  "}`, withCookie(gm)); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "bad_request" {
		t.Errorf("empty name = %d, want 400 bad_request", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/campaigns", `not json`, withCookie(gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("garbage body = %d, want 400", rec.Code)
	}

	rec = f.do(http.MethodPatch, "/api/campaigns/"+id, `{"name":"Новое"}`, withCookie(gm))
	var renamed campaignJSON
	decodeBody(t, rec, &renamed)
	if rec.Code != http.StatusOK || renamed.Name != "Новое" {
		t.Errorf("rename = %d %+v", rec.Code, renamed)
	}
	if rec := f.do(http.MethodPatch, "/api/campaigns/nope", `{"name":"x"}`, withCookie(gm)); rec.Code != http.StatusNotFound || errorCode(t, rec) != "not_found" {
		t.Errorf("rename unknown = %d, want 404 not_found", rec.Code)
	}
}

func TestGameSessionsAPI(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")

	gsID, link := f.startGame(gm, campaignID)
	if !strings.HasPrefix(link, "http://example.com/play/") {
		t.Errorf("link = %q, want it to start with http://example.com/play/", link)
	}
	var list []campaignJSON
	decodeBody(t, f.do(http.MethodGet, "/api/campaigns", "", withCookie(gm)), &list)
	if len(list) != 1 || list[0].ActiveGameSession == nil || list[0].ActiveGameSession.ID != gsID {
		t.Errorf("campaigns after start = %+v, want the active game %s", list, gsID)
	}

	rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/link", "", withCookie(gm))
	var rotated struct {
		Link string `json:"link"`
	}
	decodeBody(t, rec, &rotated)
	if rec.Code != http.StatusOK || rotated.Link == "" || rotated.Link == link {
		t.Fatalf("rotate = %d %+v, want a new link", rec.Code, rotated)
	}
	if rec := f.do(http.MethodGet, "/api/game-links/"+tokenFromLink(link), ""); rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("old link = %d, want 403 no_active_game", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/game-links/"+tokenFromLink(rotated.Link), ""); rec.Code != http.StatusOK {
		t.Errorf("new link = %d, want 200", rec.Code)
	}

	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end = %d", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(gm)); rec.Code != http.StatusNotFound {
		t.Errorf("second end = %d, want 404", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/link", "", withCookie(gm)); rec.Code != http.StatusNotFound {
		t.Errorf("rotate an ended game = %d, want 404", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/campaigns/nope/game-sessions", "", withCookie(gm)); rec.Code != http.StatusNotFound {
		t.Errorf("start in an unknown campaign = %d, want 404", rec.Code)
	}
}

func TestGameLinkUsesRequestScheme(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")

	rec := f.do(http.MethodPost, "/api/campaigns/"+campaignID+"/game-sessions", "", withCookie(gm), withTLS)
	var g startedGameJSON
	decodeBody(t, rec, &g)
	if !strings.HasPrefix(g.Link, "https://example.com/play/") {
		t.Errorf("link = %q, want https://example.com/play/…", g.Link)
	}
}

func TestPlayersAPI(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")
	_, link := f.startGame(gm, campaignID)
	_, me := f.playerSession(link, "bob")

	rec := f.do(http.MethodGet, "/api/campaigns/"+campaignID+"/players", "", withCookie(gm))
	var players []playerJSON
	decodeBody(t, rec, &players)
	if rec.Code != http.StatusOK || len(players) != 2 {
		t.Fatalf("players = %d %+v, want 2", rec.Code, players)
	}
	if players[0].Role != "gm" || players[0].AccountLogin != gmLogin {
		t.Errorf("players[0] = %+v, want the GM", players[0])
	}
	if p := players[1]; p.Role != "player" || p.AccountLogin != "bob" || p.AccountID == "" || p.ID != me.Player.ID || p.Removed {
		t.Errorf("players[1] = %+v", p)
	}
	if strings.Contains(rec.Body.String(), `"online"`) {
		t.Error("the list must not have an online field before websockets exist")
	}
	if rec := f.do(http.MethodGet, "/api/campaigns/nope/players", "", withCookie(gm)); rec.Code != http.StatusNotFound {
		t.Errorf("players of an unknown campaign = %d, want 404", rec.Code)
	}

	if rec := f.do(http.MethodPost, "/api/players/"+me.Player.ID+"/remove", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("remove = %d", rec.Code)
	}
	decodeBody(t, f.do(http.MethodGet, "/api/campaigns/"+campaignID+"/players", "", withCookie(gm)), &players)
	if !players[1].Removed {
		t.Errorf("players[1] = %+v, want Removed", players[1])
	}
	rec = f.do(http.MethodPost, "/api/players/"+players[0].ID+"/remove", "", withCookie(gm))
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "conflict" {
		t.Errorf("remove the GM = %d, want 409 conflict", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/players/nope/remove", "", withCookie(gm)); rec.Code != http.StatusNotFound {
		t.Errorf("remove unknown = %d, want 404", rec.Code)
	}
}

func TestGameLinkIsNotLogged(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")
	gsID, link := f.startGame(gm, campaignID)
	rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/link", "", withCookie(gm))
	var rotated struct {
		Link string `json:"link"`
	}
	decodeBody(t, rec, &rotated)
	f.do(http.MethodGet, "/api/game-links/"+tokenFromLink(rotated.Link), "")

	logs := f.logs.String()
	if !strings.Contains(logs, "game session started") {
		t.Fatalf("logs lack the start line: %q", logs)
	}
	for _, token := range []string{tokenFromLink(link), tokenFromLink(rotated.Link)} {
		if strings.Contains(logs, token) {
			t.Errorf("logs contain a game link token")
		}
	}
}
