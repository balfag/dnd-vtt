package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/wiki/doc"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

// uploadImage stores a PNG of the given width in the campaign.
func (tb *table) uploadImage(campaignID string, width int) asset.Asset {
	tb.t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, 4))); err != nil {
		tb.t.Fatal(err)
	}
	a, _, err := asset.NewService(tb.db, tb.t.TempDir(), asset.Options{}).Upload(context.Background(), campaignID, "i.png", "handout", &buf)
	if err != nil {
		tb.t.Fatal(err)
	}
	return a
}

func handoutDoc(text string, assetIDs ...string) json.RawMessage {
	var blocks []string
	blocks = append(blocks, fmt.Sprintf(`{"type":"paragraph","content":[{"type":"text","text":%q}]}`, text))
	for _, id := range assetIDs {
		blocks = append(blocks, fmt.Sprintf(`{"type":"image","attrs":{"assetId":%q}}`, id))
	}
	return json.RawMessage(`{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`)
}

func (tb *table) handoutAssetRows(messageID string) []string {
	tb.t.Helper()
	rows, err := tb.db.Query(`SELECT asset_id FROM chat_message_asset WHERE message_id = ? ORDER BY asset_id`, messageID)
	if err != nil {
		tb.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			tb.t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func TestSendHandout(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	c, gs := tb.camp.ID, tb.gs.ID
	img := tb.uploadImage(c, 5)

	t.Run("public", func(t *testing.T) {
		m, err := tb.svc.SendHandout(ctx, c, gs, tb.gm.playerID, "  Old map  ", handoutDoc("Hi", img.ID), nil)
		if err != nil {
			t.Fatal(err)
		}
		if m.Kind != "handout" || m.Visibility != "public" || m.Handout == nil || m.Handout.Title != "Old map" ||
			m.AuthorPlayerID == nil || *m.AuthorPlayerID != tb.gm.playerID || len(m.To) != 0 {
			t.Errorf("message = %+v", m)
		}
		if got := tb.handoutAssetRows(m.ID); !slices.Equal(got, []string{img.ID}) {
			t.Errorf("chat_message_asset = %v, want [%s]", got, img.ID)
		}
		if err := asset.NewService(tb.db, t.TempDir(), asset.Options{}).Delete(ctx, img.ID); !errors.Is(err, asset.ErrInUse) {
			t.Errorf("Delete of a handout asset = %v, want ErrInUse", err)
		}
	})

	t.Run("private without a participant row", func(t *testing.T) {
		m, err := tb.svc.SendHandout(ctx, c, gs, "", "", handoutDoc("Secret"), []string{tb.anna.playerID})
		if err != nil {
			t.Fatal(err)
		}
		if m.Visibility != "private" || m.AuthorPlayerID != nil || !slices.Equal(m.To, []string{tb.anna.playerID}) {
			t.Errorf("message = %+v", m)
		}
		var author *string
		if err := tb.db.QueryRow(`SELECT author_player_id FROM chat_message WHERE id = ?`, m.ID).Scan(&author); err != nil || author != nil {
			t.Errorf("author_player_id = %v, %v; want NULL", author, err)
		}
		if got := tb.handoutAssetRows(m.ID); len(got) != 0 {
			t.Errorf("chat_message_asset = %v, want none", got)
		}
	})

	other, err := tb.game.CreateCampaign(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	foreign := tb.uploadImage(other.ID, 6)
	twentyOne := make([]string, 21)
	for i := range twentyOne {
		twentyOne[i] = img.ID
	}

	for _, tt := range []struct {
		name    string
		title   string
		content json.RawMessage
		to      []string
		want    error
	}{
		{"mention", "t", json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"kind":"page","id":"01J00000000000000000000PG1"}}]}]}`), nil, ErrInvalid},
		{"21 images", "t", handoutDoc("x", twentyOne...), nil, ErrInvalid},
		{"asset of another campaign", "t", handoutDoc("x", foreign.ID), nil, ErrInvalid},
		{"title of 201 characters", strings.Repeat("я", 201), handoutDoc("x"), nil, ErrInvalid},
		{"malformed content", "t", json.RawMessage(`{`), nil, ErrInvalid},
		{"the GM as a recipient", "t", handoutDoc("x"), []string{tb.gm.playerID}, ErrInvalid},
		{"duplicate recipient", "t", handoutDoc("x"), []string{tb.anna.playerID, tb.anna.playerID}, ErrInvalid},
		{"unknown recipient", "t", handoutDoc("x"), []string{"nobody"}, ErrNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := tb.messageCount()
			if _, err := tb.svc.SendHandout(ctx, c, gs, tb.gm.playerID, tt.title, tt.content, tt.to); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tb.messageCount() != before {
				t.Error("a rejected handout was stored")
			}
		})
	}

	t.Run("title of 200 characters", func(t *testing.T) {
		if _, err := tb.svc.SendHandout(ctx, c, gs, tb.gm.playerID, strings.Repeat("я", 200), handoutDoc("x"), nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestHandoutRender(t *testing.T) {
	a := "P-author"
	m := protocol.ChatMessage{
		AuthorPlayerID: &a, Kind: kindHandout, Visibility: visPrivate, To: []string{"P-addressee"},
		Handout: &protocol.ChatHandout{Title: "t", Content: json.RawMessage(`{}`)},
	}
	for name, tc := range map[string]struct {
		v    ws.Viewer
		want bool
	}{
		"addressee": {ws.Viewer{PlayerID: "P-addressee"}, true},
		"author":    {ws.Viewer{PlayerID: a}, true},
		"gm":        {ws.Viewer{IsGM: true, PlayerID: "P-gm"}, true},
		"gm-nobody": {ws.Viewer{IsGM: true}, true},
		"other":     {ws.Viewer{PlayerID: "P-other"}, false},
		"nobody":    {ws.Viewer{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := Render(m, tc.v); ok != tc.want {
				t.Errorf("visible = %v, want %v", ok, tc.want)
			}
		})
	}
	m.Visibility, m.To = visPublic, nil
	if _, ok := Render(m, ws.Viewer{PlayerID: "P-other"}); !ok {
		t.Error("a public handout must be visible to everyone")
	}
}

func TestHandoutShowCommand(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	gm, anna, boris, carl := tb.everyone()
	img := tb.uploadImage(tb.camp.ID, 7)
	body := func(to ...string) string {
		d, err := json.Marshal(protocol.HandoutShow{Title: "Letter", Content: handoutDoc("Dear", img.ID), To: to})
		if err != nil {
			t.Fatal(err)
		}
		return string(d)
	}

	command(t, anna, "handout.show", "c-1", body())
	wantErr(t, recv(t, anna), "c-1", protocol.CodeForbidden)
	if tb.messageCount() != 0 {
		t.Error("a player stored a handout")
	}

	command(t, gm, "handout.show", "c-2", body(tb.boris.playerID))
	ev := recv(t, gm)
	m := wantMessage(t, ev)
	wantAck(t, recv(t, gm), "c-2")
	if m.Kind != "handout" || m.Visibility != "private" || m.Handout == nil || m.Handout.Title != "Letter" {
		t.Errorf("gm's message = %+v", m)
	}
	if got := wantMessage(t, recv(t, boris)); got.ID != m.ID || got.Handout == nil {
		t.Errorf("boris got %+v", got)
	}
	wantNoop(t, recv(t, anna), ev.Seq)
	wantNoop(t, recv(t, carl), ev.Seq)

	command(t, gm, "handout.show", "c-3", body())
	pub := wantMessage(t, recv(t, gm))
	wantAck(t, recv(t, gm), "c-3")
	for name, c := range map[string]*websocket.Conn{"anna": anna, "boris": boris, "carl": carl} {
		if got := wantMessage(t, recv(t, c)); got.ID != pub.ID {
			t.Errorf("%s got %s, want %s", name, got.ID, pub.ID)
		}
	}

	// The third command within a second exceeds the limit of 2 per second.
	command(t, gm, "handout.show", "c-4", body())
	wantErr(t, recv(t, gm), "c-4", protocol.CodeRateLimited)
}

func TestHandoutShowRejectsLargeAndBadContent(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	gm, _, _, _ := tb.everyone()

	d, err := json.Marshal(protocol.HandoutShow{Title: "x", Content: handoutDoc(strings.Repeat("a", doc.MaxHandoutBytes))})
	if err != nil {
		t.Fatal(err)
	}
	command(t, gm, "handout.show", "c-1", string(d))
	wantErr(t, recv(t, gm), "c-1", protocol.CodeBadRequest)

	command(t, gm, "handout.show", "c-2", `{"title":"x","content":{"type":"doc","content":[]},"extra":1}`)
	wantErr(t, recv(t, gm), "c-2", protocol.CodeBadRequest)
	if tb.messageCount() != 0 {
		t.Error("a rejected handout was stored")
	}
}

func TestHandoutShowWithoutActiveGame(t *testing.T) {
	tb := newTable(t)
	tb.register(&seqRoller{t: t})
	if err := tb.game.EndGameSession(context.Background(), tb.gs.ID); err != nil {
		t.Fatal(err)
	}
	c := tb.dial(tb.gm)
	send(t, c, `{"t":"sync.hello","d":{}}`)
	if env := recv(t, c); env.T != protocol.TypeSnapshot {
		t.Fatalf("got %+v, want sync.snapshot", env)
	}
	command(t, c, "handout.show", "c-1", `{"title":"x","content":{"type":"doc","content":[{"type":"paragraph"}]}}`)
	wantErr(t, recv(t, c), "c-1", protocol.CodeConflict)
}

func TestHandoutInHistoryAndSnapshot(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	c, gs := tb.camp.ID, tb.gs.ID
	if _, err := tb.svc.SendText(ctx, c, gs, tb.anna.playerID, "first", nil); err != nil {
		t.Fatal(err)
	}
	pub, err := tb.svc.SendHandout(ctx, c, gs, tb.gm.playerID, "Public", handoutDoc("p"), nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := tb.svc.SendHandout(ctx, c, gs, tb.gm.playerID, "Private", handoutDoc("q"), []string{tb.boris.playerID})
	if err != nil {
		t.Fatal(err)
	}
	// A GM without a participant row still sees the private handout of the GM.
	last, err := tb.svc.SendText(ctx, c, gs, tb.anna.playerID, "last", nil)
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		v    ws.Viewer
		want []string // handout ids in the history
	}{
		"boris": {ws.Viewer{PlayerID: tb.boris.playerID}, []string{pub.ID, priv.ID}},
		"anna":  {ws.Viewer{PlayerID: tb.anna.playerID}, []string{pub.ID}},
		"gm":    {ws.Viewer{IsGM: true, PlayerID: tb.gm.playerID}, []string{pub.ID, priv.ID}},
		"gm-np": {ws.Viewer{IsGM: true}, []string{pub.ID, priv.ID}},
	} {
		t.Run(name, func(t *testing.T) {
			hist, err := tb.svc.History(ctx, tc.v, c, last.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			var table protocol.SnapshotTable
			if err := tb.svc.FillSnapshot(ctx, tc.v, c, &table); err != nil {
				t.Fatal(err)
			}
			for where, msgs := range map[string][]protocol.ChatMessage{"history": hist, "snapshot": table.Chat} {
				var got []string
				for _, m := range msgs {
					if m.Kind == "handout" {
						if m.Handout == nil || m.Handout.Title == "" || len(m.Handout.Content) == 0 {
							t.Errorf("%s: handout without a payload: %+v", where, m)
						}
						got = append(got, m.ID)
					}
				}
				if !slices.Equal(got, tc.want) {
					t.Errorf("%s: handouts = %v, want %v", where, got, tc.want)
				}
			}
		})
	}
}

func TestPlayerCanSeeHandoutAsset(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	c, gs := tb.camp.ID, tb.gs.ID
	pubImg, privImg, none := tb.uploadImage(c, 5), tb.uploadImage(c, 6), tb.uploadImage(c, 7)
	if _, err := tb.svc.SendHandout(ctx, c, gs, "", "a", handoutDoc("p", pubImg.ID), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.svc.SendHandout(ctx, c, gs, "", "b", handoutDoc("q", privImg.ID), []string{tb.boris.playerID}); err != nil {
		t.Fatal(err)
	}
	other, err := tb.game.CreateCampaign(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name             string
		campaign, player string
		asset            string
		want             bool
	}{
		{"public, anna", c, tb.anna.playerID, pubImg.ID, true},
		{"private, addressee", c, tb.boris.playerID, privImg.ID, true},
		{"private, another player", c, tb.anna.playerID, privImg.ID, false},
		{"never shown", c, tb.boris.playerID, none.ID, false},
		{"another campaign", other.ID, tb.boris.playerID, pubImg.ID, false},
	} {
		got, err := tb.svc.PlayerCanSeeHandoutAsset(ctx, tt.campaign, tt.player, tt.asset)
		if err != nil || got != tt.want {
			t.Errorf("%s = %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
}
