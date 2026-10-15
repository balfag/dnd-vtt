package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// resetFixture is a GM with a game and one player ("bob").
type resetFixture struct {
	*apiFixture
	gm         *http.Cookie
	campaignID string
	link       string // the game link
	bob        *http.Cookie
	bobID      string // bob's account id
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	f := newAPIFixture(t)
	r := &resetFixture{apiFixture: f, gm: f.gmCookie()}
	r.campaignID = f.createCampaign(r.gm, "Camp")
	_, r.link = f.startGame(r.gm, r.campaignID)
	r.bob, _ = f.playerSession(r.link, "bob")

	rec := f.do(http.MethodGet, "/api/campaigns/"+r.campaignID+"/players", "", withCookie(r.gm))
	var players []playerJSON
	decodeBody(t, rec, &players)
	for _, p := range players {
		if p.AccountLogin == "bob" {
			r.bobID = p.AccountID
		}
	}
	if r.bobID == "" {
		t.Fatalf("bob is not in the players list: %+v", players)
	}
	return r
}

// issue creates a reset link for bob and returns the response, the link and its token.
func (r *resetFixture) issue() (*httptest.ResponseRecorder, string, string) {
	r.t.Helper()
	rec := r.do(http.MethodPost, "/api/accounts/"+r.bobID+"/password-resets", "", withCookie(r.gm))
	var body struct {
		Link string `json:"link"`
	}
	if rec.Code == http.StatusCreated {
		decodeBody(r.t, rec, &body)
	}
	_, token, _ := strings.Cut(body.Link, "/reset/")
	return rec, body.Link, token
}

func (r *resetFixture) apply(token, password string) *httptest.ResponseRecorder {
	r.t.Helper()
	return r.do(http.MethodPost, "/api/password-resets/"+token, jsonBody(r.t, map[string]string{"new": password}))
}

func TestCreatePasswordReset(t *testing.T) {
	f := newResetFixture(t)

	rec, link, token := f.issue()
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(link, "http://example.com/reset/") || token == "" {
		t.Errorf("link = %q, want http://example.com/reset/<token>", link)
	}
	var body struct {
		ExpiresAt int64 `json:"expiresAt"`
	}
	decodeBody(t, rec, &body)
	if want := f.clock.Add(auth.PasswordResetTTL).UnixMilli(); body.ExpiresAt != want {
		t.Errorf("expiresAt = %d, want %d (now + 30 min)", body.ExpiresAt, want)
	}

	path := "/api/accounts/" + f.bobID + "/password-resets"
	if rec := f.do(http.MethodPost, path, "", withCookie(f.bob)); rec.Code != http.StatusForbidden {
		t.Errorf("player = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodPost, path, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no cookie = %d, want 401", rec.Code)
	}
	var me meResponse
	decodeBody(t, f.do(http.MethodGet, "/api/auth/me", "", withCookie(f.gm)), &me)
	rec = f.do(http.MethodPost, "/api/accounts/"+me.AccountID+"/password-resets", "", withCookie(f.gm))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
		t.Errorf("gm account = %d %s, want 403 forbidden", rec.Code, rec.Body.String())
	}
	if rec := f.do(http.MethodPost, "/api/accounts/nope/password-resets", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown account = %d, want 404", rec.Code)
	}
}

func TestApplyPasswordResetByLink(t *testing.T) {
	f := newResetFixture(t)
	_, _, token := f.issue()

	if rec := f.apply(token, "short"); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password = %d, want 400", rec.Code)
	}
	if rec := f.apply(token, "brand-new-password"); rec.Code != http.StatusNoContent {
		t.Fatalf("apply = %d %s, want 204 (a typo must not burn the link)", rec.Code, rec.Body.String())
	}
	if rec := f.apply(token, "another-password"); rec.Code != http.StatusForbidden {
		t.Errorf("second use = %d, want 403", rec.Code)
	}

	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(f.bob)); rec.Code != http.StatusUnauthorized {
		t.Errorf("old session = %d, want 401", rec.Code)
	}
	login := func(password string) int {
		return f.do(http.MethodPost, "/api/auth/login", jsonBody(t, map[string]string{
			"login": "bob", "password": password, "gameLink": tokenFromLink(f.link),
		})).Code
	}
	if code := login(playerPassword); code != http.StatusUnauthorized {
		t.Errorf("old password = %d, want 401", code)
	}
	if code := login("brand-new-password"); code != http.StatusOK {
		t.Errorf("new password = %d, want 200", code)
	}
}

func TestApplyPasswordResetClosesWebSocket(t *testing.T) {
	f := newResetFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)
	_, _, token := f.issue()

	ws := connectWS(t, srv, f.bob)
	if rec := f.apply(token, "brand-new-password"); rec.Code != http.StatusNoContent {
		t.Fatalf("apply = %d", rec.Code)
	}
	ws.wantClosed(t, websocket.StatusCode(protocol.CloseSessionRevoked))
}

func TestApplyPasswordResetRateLimit(t *testing.T) {
	f := newResetFixture(t)

	for i := range limitPerIP {
		if rec := f.apply("garbage-token", "brand-new-password"); rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d = %d, want 403", i+1, rec.Code)
		}
	}
	rec := f.apply("garbage-token", "brand-new-password")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("attempt %d = %d, want 429 with Retry-After", limitPerIP+1, rec.Code)
	}
}

func TestPasswordResetTokenIsNotLogged(t *testing.T) {
	f := newResetFixture(t)
	_, link, token := f.issue()
	if rec := f.apply(token, "brand-new-password"); rec.Code != http.StatusNoContent {
		t.Fatalf("apply = %d", rec.Code)
	}

	logs := f.logs.String()
	if strings.Contains(logs, token) || strings.Contains(logs, link) {
		t.Errorf("logs contain the reset token or link:\n%s", logs)
	}
	for _, want := range []string{"password reset issued", "password reset applied", f.bobID} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs lack %q:\n%s", want, logs)
		}
	}
}
