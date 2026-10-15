package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func TestWSRequiresLogin(t *testing.T) {
	f := newAPIFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var e errorResponse
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Code != "unauthorized" {
		t.Fatalf("body %q, want error.code unauthorized", body)
	}
}

func TestWSServesSnapshotToGM(t *testing.T) {
	f := newAPIFixture(t)
	cookie := f.gmCookie()
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, dialResp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if dialResp.Body != nil {
		_ = dialResp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })

	if err := c.Write(ctx, websocket.MessageText, []byte(`{"t":"sync.hello","d":{}}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil || env.T != protocol.TypeSnapshot {
		t.Fatalf("got %s, want sync.snapshot", data)
	}
	var snap protocol.Snapshot
	if err := json.Unmarshal(env.D, &snap); err != nil || !snap.Me.IsGM {
		t.Fatalf("snapshot %s: %v", env.D, err)
	}
}

// wsClient is a WebSocket connection of a test. After hello it reads in the
// background, because the server waits for the close frame of the client.
type wsClient struct {
	c      *websocket.Conn
	status chan websocket.StatusCode // the close code, once the server closes the connection
}

// connectWS dials /ws with cookie, completes sync.hello and starts reading.
func connectWS(t *testing.T, srv *httptest.Server, cookie *http.Cookie) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": {cookie.String()}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"t":"sync.hello","d":{}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil { // the snapshot: the connection is registered
		t.Fatal(err)
	}

	w := &wsClient{c: c, status: make(chan websocket.StatusCode, 1)}
	go func() {
		for {
			if _, _, err := c.Read(context.Background()); err != nil {
				w.status <- websocket.CloseStatus(err)
				return
			}
		}
	}()
	return w
}

// wantClosed waits for the server to close the connection with code.
func (w *wsClient) wantClosed(t *testing.T, code websocket.StatusCode) {
	t.Helper()
	select {
	case got := <-w.status:
		if got != code {
			t.Fatalf("close = %d, want %d", got, code)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("connection was not closed, want %d", code)
	}
}

// wantOpen asserts that the connection is still open and has not been closed.
func (w *wsClient) wantOpen(t *testing.T) {
	t.Helper()
	select {
	case got := <-w.status:
		t.Fatalf("connection was closed with %d, want it open", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestLogoutClosesWebSocket(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	ws := connectWS(t, srv, gm)
	if rec := f.do(http.MethodPost, "/api/auth/logout", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	ws.wantClosed(t, websocket.StatusCode(protocol.CloseSessionRevoked))
}

func TestPasswordChangeClosesOtherSessionsOnly(t *testing.T) {
	f := newAPIFixture(t)
	first := f.gmCookie()
	second := sessionCookie(t, f.login(gmLogin, gmPassword))
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	wsFirst := connectWS(t, srv, first)
	wsSecond := connectWS(t, srv, second)

	rec := f.do(http.MethodPost, "/api/auth/password",
		jsonBody(t, map[string]string{"current": gmPassword, "new": "another-password"}), withCookie(first))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change password = %d %s", rec.Code, rec.Body.String())
	}
	wsSecond.wantClosed(t, websocket.StatusCode(protocol.CloseSessionRevoked))
	wsFirst.wantOpen(t)
}

func TestPlayersListReportsOnline(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")
	_, link := f.startGame(gm, campaignID)
	bob, _ := f.playerSession(link, "bob")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	connectWS(t, srv, bob)

	rec := f.do(http.MethodGet, "/api/campaigns/"+campaignID+"/players", "", withCookie(gm))
	var players []playerJSON
	decodeBody(t, rec, &players)
	if len(players) != 2 {
		t.Fatalf("players = %+v, want 2", players)
	}
	for _, p := range players {
		if want := p.AccountLogin == "bob"; p.Online != want {
			t.Errorf("%s online = %v, want %v", p.AccountLogin, p.Online, want)
		}
	}
}

func TestEndGameClosesPlayerWebSocket(t *testing.T) {
	f := newAPIFixture(t)
	gm := f.gmCookie()
	campaignID := f.createCampaign(gm, "Camp")
	gsID, link := f.startGame(gm, campaignID)
	bob, _ := f.playerSession(link, "bob")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	ws := connectWS(t, srv, bob)
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end game = %d", rec.Code)
	}
	ws.wantClosed(t, websocket.StatusCode(protocol.CloseGameEnded))
}
