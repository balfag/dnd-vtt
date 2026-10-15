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
