package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/dice"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func TestRender(t *testing.T) {
	const author, addressee, other = "P-author", "P-addressee", "P-other"
	result := &dice.Result{Formula: "1d20", Total: 7}
	msg := func(kind, vis string) protocol.ChatMessage {
		a := author
		m := protocol.ChatMessage{ID: "M1", AuthorPlayerID: &a, Kind: kind, Visibility: vis}
		if kind == kindRoll {
			m.Roll = &protocol.ChatRoll{Formula: "1d20", Result: result}
		} else {
			m.Text = "hi"
		}
		if vis == visPrivate {
			m.To = []string{addressee}
		}
		return m
	}
	viewers := map[string]ws.Viewer{
		"author":    {PlayerID: author},
		"addressee": {PlayerID: addressee},
		"other":     {PlayerID: other},
		"gm":        {IsGM: true, PlayerID: "P-gm"},
		"gm-author": {IsGM: true, PlayerID: author},
		"gm-addr":   {IsGM: true, PlayerID: addressee},
		"gm-nobody": {IsGM: true},
	}
	tests := []struct {
		kind, vis string
		want      map[string]bool // who sees the message
	}{
		{kindText, visPublic, map[string]bool{"author": true, "addressee": true, "other": true, "gm": true, "gm-author": true, "gm-addr": true, "gm-nobody": true}},
		{kindText, visPrivate, map[string]bool{"author": true, "addressee": true, "gm-author": true, "gm-addr": true}},
		{kindRoll, visGM, map[string]bool{"author": true, "gm": true, "gm-author": true, "gm-addr": true, "gm-nobody": true}},
		{kindRoll, visSelf, map[string]bool{"author": true, "gm-author": true}},
		{kindRoll, visBlind, map[string]bool{"author": true, "gm": true, "gm-author": true, "gm-addr": true, "gm-nobody": true}},
	}
	for _, tc := range tests {
		for name, v := range viewers {
			t.Run(tc.vis+"/"+name, func(t *testing.T) {
				in := msg(tc.kind, tc.vis)
				before := fmt.Sprintf("%+v %+v", in, in.Roll)
				out, ok := Render(in, v)
				if ok != tc.want[name] {
					t.Fatalf("visible = %v, want %v", ok, tc.want[name])
				}
				if after := fmt.Sprintf("%+v %+v", in, in.Roll); after != before {
					t.Errorf("Render changed its input: %s -> %s", before, after)
				}
				if !ok {
					return
				}
				if tc.vis == visBlind {
					// Only the GM sees the result of a blind roll, and the author when it is the GM.
					wantResult := v.IsGM
					if gotResult := out.Roll != nil && out.Roll.Result != nil; gotResult != wantResult {
						t.Errorf("result present = %v, want %v", gotResult, wantResult)
					}
				}
			})
		}
	}
}

// listCampaign is the SQL-free reference: ids of the messages Render shows to v.
func renderedIDs(msgs []protocol.ChatMessage, v ws.Viewer) []string {
	var ids []string
	for _, m := range msgs {
		if _, ok := Render(m, v); ok {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func TestSQLFilterAgreesWithRender(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	roller := &seqRoller{t: t, vals: make([]int, 200)}
	for i := range roller.vals {
		roller.vals[i] = 1
	}
	gs, c := tb.gs.ID, tb.camp.ID

	var all []protocol.ChatMessage
	add := func(m protocol.ChatMessage, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, m)
	}
	authors := []identity{tb.anna, tb.boris}
	for i, a := range authors {
		other := authors[1-i]
		add(tb.svc.SendText(ctx, c, gs, a.playerID, "public", nil))
		add(tb.svc.SendText(ctx, c, gs, a.playerID, "to other", []string{other.playerID}))
		add(tb.svc.SendText(ctx, c, gs, a.playerID, "to carl", []string{tb.carl.playerID}))
		for _, vis := range []string{visPublic, visGM, visSelf, visBlind} {
			add(tb.svc.SendRoll(ctx, c, gs, a.playerID, "1d20", vis, "", roller))
		}
	}

	viewers := map[string]ws.Viewer{
		"anna":  {PlayerID: tb.anna.playerID},
		"boris": {PlayerID: tb.boris.playerID},
		"carl":  {PlayerID: tb.carl.playerID},
		"gm":    {IsGM: true, PlayerID: tb.gm.playerID},
	}
	for name, v := range viewers {
		t.Run(name, func(t *testing.T) {
			want := renderedIDs(all, v)
			if len(want) == 0 || len(want) == len(all) {
				t.Fatalf("the data set does not discriminate: %d of %d visible", len(want), len(all))
			}

			// The SQL alone, before Render, must select exactly the same set.
			gm := 0
			if v.IsGM {
				gm = 1
			}
			rows, err := tb.db.QueryContext(ctx, visibleQuery, c, "", v.PlayerID, gm, 1000)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			var sqlIDs []string
			for rows.Next() {
				var id, kind, vis, recipients, payload string
				var created int64
				var author *string
				if err := rows.Scan(&id, &created, &author, &kind, &vis, &recipients, &payload); err != nil {
					t.Fatal(err)
				}
				sqlIDs = append(sqlIDs, id)
			}
			slices.Sort(sqlIDs)
			sorted := slices.Clone(want)
			slices.Sort(sorted)
			if !slices.Equal(sqlIDs, sorted) {
				t.Errorf("SQL selects %v, Render shows %v", sqlIDs, sorted)
			}

			var table protocol.SnapshotTable
			if err := tb.svc.FillSnapshot(ctx, v, c, &table); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range table.Chat {
				got = append(got, m.ID)
			}
			if !slices.Equal(got, want) {
				t.Errorf("snapshot has %v, want %v", got, want)
			}
		})
	}
}

func TestSnapshotKeepsTheLast50(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	var ids []string
	for i := 0; i < 60; i++ {
		m, err := tb.svc.SendText(ctx, tb.camp.ID, tb.gs.ID, tb.anna.playerID, fmt.Sprintf("m%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	var table protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(ctx, ws.Viewer{PlayerID: tb.boris.playerID}, tb.camp.ID, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Chat) != 50 || table.Chat[0].ID != ids[10] || table.Chat[49].ID != ids[59] {
		t.Fatalf("snapshot has %d messages from %v, want the last 50 ascending", len(table.Chat), table.Chat[0].ID)
	}
}

func TestSnapshotCountsOnlyVisibleMessages(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	// 60 messages, of which Carl sees 10: the rest is private between Anna and Boris.
	for i := 0; i < 60; i++ {
		var to []string
		if i%6 != 0 {
			to = []string{tb.boris.playerID}
		}
		if _, err := tb.svc.SendText(ctx, tb.camp.ID, tb.gs.ID, tb.anna.playerID, "x", to); err != nil {
			t.Fatal(err)
		}
	}
	var table protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(ctx, ws.Viewer{PlayerID: tb.carl.playerID}, tb.camp.ID, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Chat) != 10 {
		t.Fatalf("carl sees %d messages, want 10", len(table.Chat))
	}
}

func TestEmptySnapshotChatIsAnEmptyArray(t *testing.T) {
	tb := newTable(t)
	var table protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(context.Background(), ws.Viewer{PlayerID: tb.anna.playerID}, tb.camp.ID, &table); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"chat":[]`) {
		t.Errorf("snapshot table = %s, want chat: []", data)
	}
}

func TestHistory(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	var ids []string
	for i := 0; i < 12; i++ {
		m, err := tb.svc.SendText(ctx, tb.camp.ID, tb.gs.ID, tb.anna.playerID, fmt.Sprintf("m%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	v := ws.Viewer{PlayerID: tb.boris.playerID}

	got, err := tb.svc.History(ctx, v, tb.camp.ID, ids[8], 5)
	if err != nil {
		t.Fatal(err)
	}
	var gotIDs []string
	for _, m := range got {
		gotIDs = append(gotIDs, m.ID)
	}
	if !slices.Equal(gotIDs, ids[3:8]) {
		t.Errorf("history = %v, want %v", gotIDs, ids[3:8])
	}

	if got, err = tb.svc.History(ctx, v, tb.camp.ID, ids[0], 0); err != nil || got == nil || len(got) != 0 {
		t.Errorf("history before the first = %v, %v, want an empty non-nil slice", got, err)
	}
	if _, err = tb.svc.History(ctx, v, tb.camp.ID, ids[8], 101); !errors.Is(err, ErrInvalid) {
		t.Errorf("limit 101: err = %v, want ErrInvalid", err)
	}
	if _, err = tb.svc.History(ctx, v, tb.camp.ID, ids[8], -1); !errors.Is(err, ErrInvalid) {
		t.Errorf("limit -1: err = %v, want ErrInvalid", err)
	}

	// A message of another campaign is not a valid anchor.
	other, err := tb.game.CreateCampaign(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tb.svc.History(ctx, v, other.ID, ids[8], 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("before from another campaign: err = %v, want ErrNotFound", err)
	}
}

func TestSendTextLimits(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	anna, boris, carl := tb.anna.playerID, tb.boris.playerID, tb.carl.playerID

	// A participant of another campaign.
	other, err := tb.game.CreateCampaign(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	const foreign = "01J00000000000000000FOREIGN"
	if _, err := tb.db.Exec(
		`INSERT INTO player (id, campaign_id, account_id, display_name, color, role, created_at) VALUES (?, ?, ?, 'F', '#aabbcc', 'player', 0)`,
		foreign, other.ID, tb.anna.acc.ID); err != nil {
		t.Fatal(err)
	}
	// A removed participant.
	if err := tb.game.RemovePlayer(ctx, carl); err != nil {
		t.Fatal(err)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprintf("P%d", i)
	}

	tests := []struct {
		name string
		text string
		to   []string
		want error // nil: accepted
	}{
		{"ok", "hi", nil, nil},
		{"private ok", "hi", []string{boris}, nil},
		{"empty", "", nil, ErrInvalid},
		{"spaces only", "  \n\t ", nil, ErrInvalid},
		{"2000 runes", strings.Repeat("я", 2000), nil, nil},
		{"2001 runes", strings.Repeat("я", 2001), nil, ErrInvalid},
		{"duplicate", "hi", []string{boris, boris}, ErrInvalid},
		{"author in to", "hi", []string{anna}, ErrInvalid},
		{"21 recipients", "hi", many, ErrInvalid},
		{"unknown", "hi", []string{"01J0000000000000000UNKNOWN"}, ErrNotFound},
		{"removed", "hi", []string{carl}, ErrNotFound},
		{"other campaign", "hi", []string{foreign}, ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := tb.svc.SendText(ctx, tb.camp.ID, tb.gs.ID, anna, tc.text, tc.to)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if err == nil {
				if m.ID == "" || m.CreatedAt == 0 || strings.TrimSpace(m.Text) != m.Text {
					t.Errorf("message = %+v", m)
				}
			}
		})
	}
}

func TestSendRollLimits(t *testing.T) {
	ctx := context.Background()
	tb := newTable(t)
	roll := func(formula, vis, label string) error {
		_, err := tb.svc.SendRoll(ctx, tb.camp.ID, tb.gs.ID, tb.anna.playerID, formula, vis, label, &seqRoller{t: t, vals: []int{1, 1, 1, 1}})
		return err
	}
	for _, vis := range []string{"public", "gm", "self", "blind"} {
		if err := roll("1d20", vis, ""); err != nil {
			t.Errorf("visibility %s: %v", vis, err)
		}
	}
	for _, vis := range []string{"", "private", "system", "PUBLIC"} {
		if err := roll("1d20", vis, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("visibility %q: err = %v, want ErrInvalid", vis, err)
		}
	}
	if err := roll("1d20", "public", strings.Repeat("я", 100)); err != nil {
		t.Errorf("label of 100 runes: %v", err)
	}
	if err := roll("1d20", "public", strings.Repeat("я", 101)); !errors.Is(err, ErrInvalid) {
		t.Errorf("label of 101 runes: err = %v, want ErrInvalid", err)
	}
	var perr *dice.ParseError
	if err := roll("2d6+", "public", ""); !errors.As(err, &perr) {
		t.Errorf("bad formula: err = %v, want *dice.ParseError", err)
	}

	m, err := tb.svc.SendRoll(ctx, tb.camp.ID, tb.gs.ID, tb.anna.playerID, " 2D6 + 3 ", "public", "  Hit  ", &seqRoller{t: t, vals: []int{2, 5}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Roll.Formula != "2d6+3" || m.Roll.Label != "Hit" || m.Roll.Result.Total != 10 {
		t.Errorf("roll = %+v %+v", m.Roll, m.Roll.Result)
	}

	// The stored roll reads back the same.
	var table protocol.SnapshotTable
	if err := tb.svc.FillSnapshot(ctx, ws.Viewer{PlayerID: tb.boris.playerID}, tb.camp.ID, &table); err != nil {
		t.Fatal(err)
	}
	last := table.Chat[len(table.Chat)-1]
	if !reflect.DeepEqual(last, m) {
		t.Errorf("stored message = %+v, want %+v", last, m)
	}
}
