package httpapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
	"github.com/balfag/dnd-vtt/internal/store"
	"github.com/balfag/dnd-vtt/internal/ws"
)

const (
	gmLogin    = "master"
	gmPassword = "master-password"
)

type apiFixture struct {
	t     *testing.T
	h     http.Handler
	svc   *auth.Service
	game  *game.Service
	db    *sql.DB
	clock time.Time
	logs  *bytes.Buffer
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	f := &apiFixture{
		t:     t,
		db:    st.DB(),
		clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		logs:  &bytes.Buffer{},
	}
	now := func() time.Time { return f.clock }
	f.svc, err = auth.NewService(f.db, auth.Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	f.game = game.NewService(f.db, game.Options{Now: now})
	logger := slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.h = NewRouter(Deps{
		Logger:  logger,
		Version: "test",
		SPA:     fstest.MapFS{},
		Auth:    f.svc,
		Game:    f.game,
		Hub:     ws.NewHub(f.game, logger, ws.Options{}),
		Now:     now,
	})
	return f
}

// do sends a request from 127.0.0.1 unless opts change it. JSON bodies get
// the Content-Type header automatically.
func (f *apiFixture) do(method, path, body string, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5555"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	// Every error response must use the common JSON envelope.
	if rec.Code >= 400 {
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			f.t.Errorf("%s %s: error Content-Type = %q", method, path, ct)
		}
		var e errorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code == "" {
			f.t.Errorf("%s %s: error body %q has no error.code", method, path, rec.Body.String())
		}
	}
	return rec
}

func remote(r *http.Request)                        { r.RemoteAddr = "192.0.2.1:1234" }
func withCookie(c *http.Cookie) func(*http.Request) { return func(r *http.Request) { r.AddCookie(c) } }
func withHeader(k, v string) func(*http.Request)    { return func(r *http.Request) { r.Header.Set(k, v) } }

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in response (status %d, body %q)", sessionCookieName, rec.Code, rec.Body.String())
	return nil
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("not an error body: %q", rec.Body.String())
	}
	return e.Error.Code
}

func (f *apiFixture) setupToken() string {
	f.t.Helper()
	tok, err := f.svc.IssueSetupToken()
	if err != nil {
		f.t.Fatal(err)
	}
	return tok
}

// createGM runs POST /api/setup and returns the response.
func (f *apiFixture) createGM() *httptest.ResponseRecorder {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/setup", jsonBody(f.t, map[string]string{
		"setupToken": f.setupToken(), "login": gmLogin, "password": gmPassword,
	}))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("setup status = %d, body %s", rec.Code, rec.Body.String())
	}
	return rec
}

func (f *apiFixture) login(login, password string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(http.MethodPost, "/api/auth/login", jsonBody(f.t, map[string]string{"login": login, "password": password}))
}

func (f *apiFixture) insertPlayer(login, password string) {
	f.t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, 0, 1, 1)`,
		"player-"+login, login, hash); err != nil {
		f.t.Fatal(err)
	}
}

func TestSetup(t *testing.T) {
	f := newAPIFixture(t)

	rec := f.do(http.MethodGet, "/api/setup", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"needed":true}` {
		t.Fatalf("GET setup = %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(http.MethodGet, "/api/setup", "", remote); rec.Code != http.StatusForbidden {
		t.Errorf("GET setup from a remote host = %d, want 403", rec.Code)
	}

	token := f.setupToken()
	body := jsonBody(t, map[string]string{"setupToken": token, "login": gmLogin, "password": gmPassword})
	if rec := f.do(http.MethodPost, "/api/setup", jsonBody(t, map[string]string{
		"setupToken": "wrong", "login": gmLogin, "password": gmPassword,
	})); rec.Code != http.StatusForbidden {
		t.Errorf("wrong token = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/setup", body, remote); rec.Code != http.StatusForbidden {
		t.Errorf("right token from a remote host = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/setup", body, withHeader("X-Forwarded-For", "203.0.113.5")); rec.Code != http.StatusForbidden {
		t.Errorf("right token with X-Forwarded-For = %d, want 403", rec.Code)
	}

	rec = f.do(http.MethodPost, "/api/setup", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	var me struct {
		Login string `json:"login"`
		IsGM  bool   `json:"isGm"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || !me.IsGM || me.Login != gmLogin {
		t.Errorf("me = %+v, %v", me, err)
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Secure {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Lax, Path=/, not Secure", c)
	}

	rec = f.do(http.MethodGet, "/api/setup", "")
	if strings.TrimSpace(rec.Body.String()) != `{"needed":false}` {
		t.Errorf("GET setup after creation = %s", rec.Body.String())
	}
	rec = f.do(http.MethodPost, "/api/setup", jsonBody(t, map[string]string{
		"setupToken": f.setupToken(), "login": "other", "password": gmPassword,
	}))
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "conflict" {
		t.Errorf("second setup = %d %s, want 409 conflict", rec.Code, rec.Body.String())
	}
}

func TestSetupValidation(t *testing.T) {
	f := newAPIFixture(t)
	token := f.setupToken()
	for _, tt := range []struct{ name, login, password string }{
		{"bad login", "a b", gmPassword},
		{"short password", gmLogin, "short"},
	} {
		rec := f.do(http.MethodPost, "/api/setup", jsonBody(t, map[string]string{
			"setupToken": token, "login": tt.login, "password": tt.password,
		}))
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "bad_request" {
			t.Errorf("%s = %d %s, want 400 bad_request", tt.name, rec.Code, rec.Body.String())
		}
	}
	if rec := f.do(http.MethodPost, "/api/setup", `not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("garbage body = %d, want 400", rec.Code)
	}
}

func TestSecureCookieOverTLS(t *testing.T) {
	f := newAPIFixture(t)
	rec := f.do(http.MethodPost, "/api/setup", jsonBody(t, map[string]string{
		"setupToken": f.setupToken(), "login": gmLogin, "password": gmPassword,
	}), func(r *http.Request) { r.TLS = &tls.ConnectionState{} })
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	if c := sessionCookie(t, rec); !c.Secure {
		t.Error("cookie over TLS must be Secure")
	}
}

func TestGMToken(t *testing.T) {
	f := newAPIFixture(t)
	f.createGM()

	token, err := f.svc.IssueGMToken()
	if err != nil {
		t.Fatal(err)
	}
	body := jsonBody(t, map[string]string{"token": token})

	if rec := f.do(http.MethodPost, "/api/auth/gm-token", body, remote); rec.Code != http.StatusForbidden {
		t.Errorf("from a remote host = %d, want 403", rec.Code)
	}
	rec := f.do(http.MethodPost, "/api/auth/gm-token", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("redeem = %d %s", rec.Code, rec.Body.String())
	}
	sessionCookie(t, rec)

	rec = f.do(http.MethodPost, "/api/auth/gm-token", body)
	if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthorized" {
		t.Errorf("reused token = %d %s, want 401 unauthorized", rec.Code, rec.Body.String())
	}
}

func TestLogin(t *testing.T) {
	f := newAPIFixture(t)
	f.createGM()
	f.insertPlayer("bob", "bob-password")

	rec := f.login(gmLogin, gmPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d %s", rec.Code, rec.Body.String())
	}
	sessionCookie(t, rec)
	if !strings.Contains(rec.Body.String(), `"isGm":true`) {
		t.Errorf("body = %s, want isGm:true", rec.Body.String())
	}

	wrong := f.login(gmLogin, "wrong-password")
	unknown := f.login("nobody", "wrong-password")
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("wrong = %d, unknown = %d; want 401 both", wrong.Code, unknown.Code)
	}
	if wrong.Body.String() != unknown.Body.String() || errorCode(t, wrong) != "invalid_credentials" {
		t.Errorf("bodies differ or wrong code: %q vs %q", wrong.Body.String(), unknown.Body.String())
	}

	rec = f.login("bob", "bob-password")
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("player login = %d %s, want 403 no_active_game", rec.Code, rec.Body.String())
	}

	rec = f.do(http.MethodPost, "/api/auth/login", jsonBody(t, map[string]string{
		"login": gmLogin, "password": gmPassword, "gameLink": "abc",
	}))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("login with gameLink = %d %s, want 403 no_active_game", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimits(t *testing.T) {
	f := newAPIFixture(t)
	f.createGM()

	for i := range 5 {
		if rec := f.login(gmLogin, "wrong-password"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, rec.Code)
		}
	}
	// Blocked even with the right password.
	rec := f.login(gmLogin, gmPassword)
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "rate_limited" {
		t.Fatalf("6th attempt = %d %s, want 429 rate_limited", rec.Code, rec.Body.String())
	}
	if secs, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || secs < 1 {
		t.Errorf("Retry-After = %q, want an integer >= 1", rec.Header().Get("Retry-After"))
	}

	f.clock = f.clock.Add(time.Minute)
	if rec := f.login(gmLogin, gmPassword); rec.Code != http.StatusOK {
		t.Errorf("after the window = %d, want 200", rec.Code)
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	f := newAPIFixture(t)
	f.createGM()

	for i := range 20 {
		if rec := f.login("user"+strconv.Itoa(i), "wrong-password"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, rec.Code)
		}
	}
	if rec := f.login("user99", "wrong-password"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("21st attempt = %d, want 429", rec.Code)
	}
}

func TestMeAndLogout(t *testing.T) {
	f := newAPIFixture(t)
	cookie := sessionCookie(t, f.createGM())

	if rec := f.do(http.MethodGet, "/api/auth/me", ""); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthorized" {
		t.Errorf("me without cookie = %d, want 401 unauthorized", rec.Code)
	}

	rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s", rec.Code, rec.Body.String())
	}
	var me map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if v, ok := me["player"]; !ok || v != nil {
		t.Errorf("player = %v (present %v), want null", v, ok)
	}
	if me["isGm"] != true || me["login"] != gmLogin {
		t.Errorf("me = %v", me)
	}

	if rec := f.do(http.MethodPost, "/api/auth/logout", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("logout without cookie = %d, want 401", rec.Code)
	}
	rec = f.do(http.MethodPost, "/api/auth/logout", "", withCookie(cookie))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if c := sessionCookie(t, rec); c.MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge = %d, want negative (Max-Age=0)", c.MaxAge)
	}
	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(cookie)); rec.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", rec.Code)
	}
}

// TestMeRefreshesCookie checks that the browser cookie follows a GM session
// the service has just extended; otherwise the GM would be signed out 30 days
// after the first sign-in even when using the app daily.
func TestMeRefreshesCookie(t *testing.T) {
	f := newAPIFixture(t)
	cookie := sessionCookie(t, f.createGM())

	// Past the renewal interval, so ResolveSession extends the session.
	f.clock = f.clock.Add(2 * time.Hour)
	rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(cookie))
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d %s", rec.Code, rec.Body.String())
	}
	got := sessionCookie(t, rec)
	if got.Value != cookie.Value {
		t.Errorf("refreshed cookie value differs from the session token")
	}
	// Cookie Expires has one-second precision.
	want := f.clock.Add(auth.GMSessionTTL).Truncate(time.Second)
	if !got.Expires.Equal(want) {
		t.Errorf("cookie Expires = %v, want %v (renewed session expiry)", got.Expires, want)
	}
}

func TestChangePassword(t *testing.T) {
	f := newAPIFixture(t)
	a := sessionCookie(t, f.createGM())
	b := sessionCookie(t, f.login(gmLogin, gmPassword))
	change := func(current, next string, c *http.Cookie) *httptest.ResponseRecorder {
		return f.do(http.MethodPost, "/api/auth/password",
			jsonBody(t, map[string]string{"current": current, "new": next}), withCookie(c))
	}

	if rec := f.do(http.MethodPost, "/api/auth/password", `{"current":"x","new":"y"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("without cookie = %d, want 401", rec.Code)
	}
	if rec := change("wrong-current", "new-password-1", a); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "invalid_credentials" {
		t.Errorf("wrong current = %d %s, want 401 invalid_credentials", rec.Code, rec.Body.String())
	}
	if rec := change(gmPassword, "short", a); rec.Code != http.StatusBadRequest {
		t.Errorf("short new password = %d, want 400", rec.Code)
	}
	if rec := change(gmPassword, "new-password-1", a); rec.Code != http.StatusNoContent {
		t.Fatalf("change = %d %s", rec.Code, rec.Body.String())
	}

	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(b)); rec.Code != http.StatusUnauthorized {
		t.Errorf("other session after change = %d, want 401", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(a)); rec.Code != http.StatusOK {
		t.Errorf("current session after change = %d, want 200", rec.Code)
	}
	if rec := f.login(gmLogin, gmPassword); rec.Code != http.StatusUnauthorized {
		t.Errorf("old password = %d, want 401", rec.Code)
	}
	if rec := f.login(gmLogin, "new-password-1"); rec.Code != http.StatusOK {
		t.Errorf("new password = %d, want 200", rec.Code)
	}
}

func TestOriginCheck(t *testing.T) {
	f := newAPIFixture(t)
	f.createGM()
	body := jsonBody(t, map[string]string{"login": gmLogin, "password": gmPassword})

	rec := f.do(http.MethodPost, "/api/auth/login", body, withHeader("Origin", "http://evil.example"))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
		t.Errorf("foreign Origin = %d %s, want 403 forbidden", rec.Code, rec.Body.String())
	}
	if rec := f.do(http.MethodPost, "/api/auth/login", body, withHeader("Origin", "http://example.com")); rec.Code != http.StatusOK {
		t.Errorf("same Origin = %d, want 200", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/auth/login", body); rec.Code != http.StatusOK {
		t.Errorf("no Origin = %d, want 200", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/auth/me", "", withHeader("Origin", "http://evil.example")); rec.Code == http.StatusForbidden {
		t.Error("GET with a foreign Origin must not be rejected by the origin check")
	}
}

func TestSecretsAreNotLogged(t *testing.T) {
	f := newAPIFixture(t)
	setup := f.createGM()
	setupToken := sessionCookie(t, setup).Value
	login := f.login(gmLogin, gmPassword)
	loginToken := sessionCookie(t, login).Value
	f.login(gmLogin, "some-wrong-password")

	logs := f.logs.String()
	if logs == "" {
		t.Fatal("expected sign-in log lines")
	}
	for _, secret := range []string{gmPassword, "some-wrong-password", setupToken, loginToken} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs contain a secret: %q", secret)
		}
	}
}

var colorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func TestGameLinkCheck(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	_, link := f.startGame(gm, f.createCampaign(gm, "Моя кампания"))

	rec := f.do(http.MethodGet, "/api/game-links/"+tokenFromLink(link), "")
	var body struct {
		CampaignName string `json:"campaignName"`
	}
	decodeBody(t, rec, &body)
	if rec.Code != http.StatusOK || body.CampaignName != "Моя кампания" {
		t.Errorf("valid link = %d %+v", rec.Code, body)
	}

	rec = f.do(http.MethodGet, "/api/game-links/garbage", "")
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("garbage link = %d, want 403 no_active_game", rec.Code)
	}
	for range 19 { // one failure is already recorded above
		f.do(http.MethodGet, "/api/game-links/garbage", "")
	}
	if rec := f.do(http.MethodGet, "/api/game-links/garbage", ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("21st failure = %d, want 429", rec.Code)
	}
}

func TestRegister(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	gsID, link := f.startGame(gm, f.createCampaign(gm, "Camp"))

	rec := f.registerPlayer(link, "bob")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rec.Code, rec.Body.String())
	}
	sessionCookie(t, rec)
	var me meResponse
	decodeBody(t, rec, &me)
	if me.IsGM || me.Player == nil || me.Player.Role != "player" || me.Player.DisplayName != "Player bob" ||
		!colorPattern.MatchString(me.Player.Color) {
		t.Errorf("me = %+v, player = %+v", me, me.Player)
	}

	if rec := f.registerPlayer(link, "bob"); rec.Code != http.StatusConflict || errorCode(t, rec) != "conflict" {
		t.Errorf("taken login = %d, want 409 conflict", rec.Code)
	}
	invalid := func(body map[string]string) *httptest.ResponseRecorder {
		body["gameLink"] = tokenFromLink(link)
		return f.do(http.MethodPost, "/api/auth/register", jsonBody(t, body))
	}
	if rec := invalid(map[string]string{"login": "carol", "password": "short", "displayName": "Carol"}); rec.Code != http.StatusBadRequest {
		t.Errorf("short password = %d, want 400", rec.Code)
	}
	if rec := invalid(map[string]string{"login": "carol", "password": playerPassword, "displayName": " "}); rec.Code != http.StatusBadRequest {
		t.Errorf("empty display name = %d, want 400", rec.Code)
	}

	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end = %d", rec.Code)
	}
	if rec := f.registerPlayer(link, "dave"); rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("register in an ended game = %d, want 403 no_active_game", rec.Code)
	}
}

func TestLoginWithGameLink(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	_, link := f.startGame(gm, f.createCampaign(gm, "Camp"))
	_, registered := f.playerSession(link, "bob")
	token := tokenFromLink(link)
	login := func(user, password, gameLink string) *httptest.ResponseRecorder {
		return f.do(http.MethodPost, "/api/auth/login", jsonBody(t, map[string]string{
			"login": user, "password": password, "gameLink": gameLink,
		}))
	}

	rec := login("bob", playerPassword, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("player login = %d %s", rec.Code, rec.Body.String())
	}
	sessionCookie(t, rec)
	var me meResponse
	decodeBody(t, rec, &me)
	if me.Player == nil || me.Player.ID != registered.Player.ID {
		t.Errorf("Me.player = %+v, want the participant from registration %s", me.Player, registered.Player.ID)
	}

	if rec := login("bob", "wrong-password", token); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "invalid_credentials" {
		t.Errorf("wrong password = %d, want 401 invalid_credentials", rec.Code)
	}

	rec = login(gmLogin, gmPassword, token)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"isGm":true`) {
		t.Errorf("GM with a valid link = %d %s", rec.Code, rec.Body.String())
	}

	// The link is checked before the password, so a bad link tells nothing about passwords.
	rec = login("bob", "wrong-password", "garbage")
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "no_active_game" {
		t.Errorf("invalid link with a wrong password = %d, want 403 no_active_game", rec.Code)
	}

	// Removed by the GM.
	if rec := f.do(http.MethodPost, "/api/players/"+registered.Player.ID+"/remove", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("remove = %d", rec.Code)
	}
	if rec := login("bob", playerPassword, token); rec.Code != http.StatusForbidden || errorCode(t, rec) != "removed" {
		t.Errorf("removed player = %d %s, want 403 removed", rec.Code, rec.Body.String())
	}
}

func TestPlayerSessionEndsWithGame(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	gsID, link := f.startGame(gm, f.createCampaign(gm, "Camp"))
	player, _ := f.playerSession(link, "bob")

	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(player)); rec.Code != http.StatusOK {
		t.Fatalf("player me before the end = %d", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end = %d", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(player)); rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthorized" {
		t.Errorf("player me after the end = %d, want 401 unauthorized", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/auth/me", "", withCookie(gm)); rec.Code != http.StatusOK {
		t.Errorf("GM me after the end = %d, want 200", rec.Code)
	}
}

func TestMePlayerOfGM(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()

	var me meResponse
	decodeBody(t, f.do(http.MethodGet, "/api/auth/me", "", withCookie(gm)), &me)
	if me.Player != nil {
		t.Errorf("Me.player without a game = %+v, want null", me.Player)
	}

	f.startGame(gm, f.createCampaign(gm, "Camp"))
	decodeBody(t, f.do(http.MethodGet, "/api/auth/me", "", withCookie(gm)), &me)
	if me.Player == nil || me.Player.Role != "gm" || me.Player.DisplayName != gmLogin {
		t.Errorf("Me.player of the GM = %+v, want role gm", me.Player)
	}
}
