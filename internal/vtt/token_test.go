package vtt

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

func fp(f float64) *float64 { return &f }
func bp(b bool) *bool       { return &b }
func ip(n int) *int         { return &n }

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// player adds a participant to the campaign.
func (e *env) player(campaign, name string, removed bool) string {
	e.t.Helper()
	id := "acc-" + name
	if _, err := e.db.Exec("INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, 0, 1, 1)",
		id, "login-"+name, "$argon2id$x"); err != nil {
		e.t.Fatal(err)
	}
	pid := "pl-" + name
	r := 0
	if removed {
		r = 1
	}
	if _, err := e.db.Exec("INSERT INTO player (id, campaign_id, account_id, display_name, color, role, removed, created_at) VALUES (?, ?, ?, ?, ?, 'player', ?, 1)",
		pid, campaign, id, name, "#aabbcc", r); err != nil {
		e.t.Fatal(err)
	}
	return pid
}

func (e *env) token(scene string, f protocol.TokenFields) protocol.Token {
	e.t.Helper()
	if f.X == nil {
		f.X = fp(100)
	}
	if f.Y == nil {
		f.Y = fp(100)
	}
	res, err := e.svc.CreateToken(context.Background(), e.camp, protocol.TokenCreate{SceneID: scene, TokenFields: f})
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Token
}

func TestCreateTokenDefaults(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	res, err := e.svc.CreateToken(context.Background(), e.camp, protocol.TokenCreate{
		SceneID: scene.SceneID, TokenFields: protocol.TokenFields{X: fp(10), Y: fp(20)},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Token
	if got.ID == "" || got.SceneID != scene.SceneID || got.Layer != "tokens" || got.X != 10 || got.Y != 20 ||
		got.W != 1 || got.H != 1 || got.Rotation != 0 || got.Z != 0 || got.Name != "" ||
		got.NameVisibility != "hover" || got.Image != nil || got.Hidden || got.Locked ||
		got.Owners == nil || len(got.Owners) != 0 || got.Bars == nil || len(got.Bars) != 0 ||
		got.Conditions == nil || len(got.Conditions) != 0 {
		t.Errorf("defaults = %+v", got)
	}
	if !res.State.Active {
		t.Error("the first scene is active, so the state must say so")
	}
	back, err := e.svc.GetToken(context.Background(), e.camp, got.ID)
	if err != nil || !jsonEqual(back, got) {
		t.Errorf("stored token = %+v, %v; want %+v", back, err, got)
	}
}

func TestTokenZGrows(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	other := e.create("Other")
	for i := range 3 {
		if got := e.token(scene.SceneID, protocol.TokenFields{}).Z; got != i {
			t.Errorf("z of token %d = %d, want %d", i, got, i)
		}
	}
	if got := e.token(other.SceneID, protocol.TokenFields{}).Z; got != 0 {
		t.Errorf("z on another scene = %d, want 0", got)
	}
	if got := e.token(scene.SceneID, protocol.TokenFields{Z: ip(40)}).Z; got != 40 {
		t.Errorf("given z = %d, want 40", got)
	}
	if got := e.token(scene.SceneID, protocol.TokenFields{}).Z; got != 41 {
		t.Errorf("z after 40 = %d, want 41", got)
	}
}

func TestCreateTokenNeedsPosition(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	for _, f := range []protocol.TokenFields{{X: fp(1)}, {Y: fp(1)}, {}} {
		_, err := e.svc.CreateToken(context.Background(), e.camp, protocol.TokenCreate{SceneID: scene.SceneID, TokenFields: f})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("fields %+v: err = %v, want ErrInvalid", f, err)
		}
	}
}

func TestTokenLimits(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern") // 2800 x 2100
	ctx := context.Background()
	player := e.player(e.camp, "ann", false)
	str := func(s string) *string { return &s }

	manyBars := func(n int) []protocol.TokenBar {
		bars := []protocol.TokenBar{}
		for i := range n {
			bars = append(bars, protocol.TokenBar{Key: "k" + string(rune('a'+i)), Value: 1, Max: 1, Visibility: "all"})
		}
		return bars
	}
	bar := func(key string, value, maxValue int, vis string) []protocol.TokenBar {
		return []protocol.TokenBar{{Key: key, Value: value, Max: maxValue, Visibility: vis}}
	}
	conds := func(n int) []string {
		out := []string{}
		for i := range n {
			out = append(out, "c"+strings.Repeat("x", i))
		}
		return out
	}

	tests := []struct {
		name string
		f    protocol.TokenFields
		ok   bool
	}{
		{"layer map", protocol.TokenFields{Layer: str("map")}, true},
		{"layer gm", protocol.TokenFields{Layer: str("gm")}, true},
		{"layer sky", protocol.TokenFields{Layer: str("sky")}, false},
		{"x at 0", protocol.TokenFields{X: fp(0)}, true},
		{"x at width", protocol.TokenFields{X: fp(2800)}, true},
		{"x below 0", protocol.TokenFields{X: fp(-0.1)}, false},
		{"x above width", protocol.TokenFields{X: fp(2800.1)}, false},
		{"y at height", protocol.TokenFields{Y: fp(2100)}, true},
		{"y above height", protocol.TokenFields{Y: fp(2100.5)}, false},
		{"w min", protocol.TokenFields{W: fp(0.25)}, true},
		{"w below min", protocol.TokenFields{W: fp(0.24)}, false},
		{"w max", protocol.TokenFields{W: fp(20)}, true},
		{"w above max", protocol.TokenFields{W: fp(20.1)}, false},
		{"h min", protocol.TokenFields{H: fp(0.25)}, true},
		{"h below min", protocol.TokenFields{H: fp(0.1)}, false},
		{"h above max", protocol.TokenFields{H: fp(21)}, false},
		{"x NaN", protocol.TokenFields{X: fp(math.NaN())}, false},
		{"y Inf", protocol.TokenFields{Y: fp(math.Inf(1))}, false},
		{"rotation NaN", protocol.TokenFields{Rotation: fp(math.NaN())}, false},
		{"rotation -Inf", protocol.TokenFields{Rotation: fp(math.Inf(-1))}, false},
		{"w NaN", protocol.TokenFields{W: fp(math.NaN())}, false},
		{"name 100 runes", protocol.TokenFields{Name: str(strings.Repeat("я", 100))}, true},
		{"name 101 runes", protocol.TokenFields{Name: str(strings.Repeat("я", 101))}, false},
		{"name of spaces", protocol.TokenFields{Name: str("   ")}, true},
		{"nameVisibility all", protocol.TokenFields{NameVisibility: str("all")}, true},
		{"nameVisibility nobody", protocol.TokenFields{NameVisibility: str("nobody")}, false},
		{"one owner", protocol.TokenFields{Owners: []string{player}}, true},
		{"repeated owner", protocol.TokenFields{Owners: []string{player, player}}, false},
		{"5 bars", protocol.TokenFields{Bars: manyBars(5)}, true},
		{"6 bars", protocol.TokenFields{Bars: manyBars(6)}, false},
		{"bar key with capital", protocol.TokenFields{Bars: bar("Hp", 1, 1, "all")}, false},
		{"bar key empty", protocol.TokenFields{Bars: bar("", 1, 1, "all")}, false},
		{"bar key 16", protocol.TokenFields{Bars: bar(strings.Repeat("a", 16), 1, 1, "all")}, true},
		{"bar key 17", protocol.TokenFields{Bars: bar(strings.Repeat("a", 17), 1, 1, "all")}, false},
		{"repeated bar key", protocol.TokenFields{Bars: append(bar("hp", 1, 1, "all"), bar("hp", 2, 2, "gm")...)}, false},
		{"bar value min", protocol.TokenFields{Bars: bar("hp", -9999, 1, "all")}, true},
		{"bar value below min", protocol.TokenFields{Bars: bar("hp", -10000, 1, "all")}, false},
		{"bar value max", protocol.TokenFields{Bars: bar("hp", 99999, 99999, "all")}, true},
		{"bar value above max", protocol.TokenFields{Bars: bar("hp", 100000, 1, "all")}, false},
		{"bar max 1", protocol.TokenFields{Bars: bar("hp", 0, 1, "all")}, true},
		{"bar max 0", protocol.TokenFields{Bars: bar("hp", 0, 0, "all")}, false},
		{"bar max above", protocol.TokenFields{Bars: bar("hp", 0, 100000, "all")}, false},
		{"bar visibility friends", protocol.TokenFields{Bars: bar("hp", 1, 1, "friends")}, false},
		{"20 conditions", protocol.TokenFields{Conditions: conds(20)}, true},
		{"21 conditions", protocol.TokenFields{Conditions: conds(21)}, false},
		{"condition with capital", protocol.TokenFields{Conditions: []string{"Prone"}}, false},
		{"condition 32", protocol.TokenFields{Conditions: []string{strings.Repeat("a", 32)}}, true},
		{"condition 33", protocol.TokenFields{Conditions: []string{strings.Repeat("a", 33)}}, false},
		{"condition with dash", protocol.TokenFields{Conditions: []string{"half-cover"}}, true},
		{"repeated condition", protocol.TokenFields{Conditions: []string{"prone", "prone"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.f
			if f.X == nil {
				f.X = fp(10)
			}
			if f.Y == nil {
				f.Y = fp(10)
			}
			_, err := e.svc.CreateToken(ctx, e.camp, protocol.TokenCreate{SceneID: scene.SceneID, TokenFields: f})
			if tt.ok && err != nil {
				t.Errorf("err = %v, want none", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestTokenOwnersLimit(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	owners := []string{}
	for i := range 21 {
		owners = append(owners, e.player(e.camp, "p"+strings.Repeat("x", i), false))
	}
	for n, ok := range map[int]bool{20: true, 21: false} {
		_, err := e.svc.CreateToken(context.Background(), e.camp, protocol.TokenCreate{
			SceneID: scene.SceneID, TokenFields: protocol.TokenFields{X: fp(1), Y: fp(1), Owners: owners[:n]},
		})
		if ok != (err == nil) || (!ok && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%d owners: err = %v", n, err)
		}
	}
}

func TestTokenNameIsTrimmed(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	name := "  Orc  "
	if got := e.token(scene.SceneID, protocol.TokenFields{Name: &name}).Name; got != "Orc" {
		t.Errorf("name = %q, want Orc", got)
	}
}

func TestTokenRotationIsNormalized(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	for in, want := range map[float64]float64{-90: 270, 720: 0, 359.5: 359.5, 360: 0, 0: 0, 450: 90, -360: 0, -1e-20: 0} {
		if got := e.token(scene.SceneID, protocol.TokenFields{Rotation: fp(in)}).Rotation; got != want {
			t.Errorf("rotation %v stored as %v, want %v", in, got, want)
		}
	}
}

func TestTokenOwnersMustBeActiveParticipantsOfTheCampaign(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	gone := e.player(e.camp, "gone", true)
	foreign := e.player(e.other, "foreign", false)
	for name, owner := range map[string]string{"removed": gone, "another campaign": foreign, "unknown": "nobody"} {
		_, err := e.svc.CreateToken(ctx, e.camp, protocol.TokenCreate{
			SceneID: scene.SceneID, TokenFields: protocol.TokenFields{X: fp(1), Y: fp(1), Owners: []string{owner}},
		})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s owner: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestUpdateTokenTellsAbsentFromEmptyLists(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	ann := e.player(e.camp, "ann", false)
	tok := e.token(scene.SceneID, protocol.TokenFields{
		Owners:     []string{ann},
		Bars:       []protocol.TokenBar{{Key: "hp", Value: 3, Max: 3, Visibility: "all"}},
		Conditions: []string{"prone"},
	})

	// Absent lists stay (decoded from the wire, as a command would be).
	var in protocol.TokenUpdate
	if err := json.Unmarshal([]byte(`{"tokenId":"`+tok.ID+`","name":"Renamed"}`), &in); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.UpdateToken(ctx, e.camp, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Token.Owners) != 1 || len(res.Token.Bars) != 1 || len(res.Token.Conditions) != 1 || res.Token.Name != "Renamed" {
		t.Errorf("absent lists changed: %+v", res.Token)
	}

	// Empty lists clear.
	in = protocol.TokenUpdate{}
	if err := json.Unmarshal([]byte(`{"tokenId":"`+tok.ID+`","owners":[],"bars":[],"conditions":[]}`), &in); err != nil {
		t.Fatal(err)
	}
	res, err = e.svc.UpdateToken(ctx, e.camp, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Token.Owners == nil || len(res.Token.Owners) != 0 || len(res.Token.Bars) != 0 || len(res.Token.Conditions) != 0 {
		t.Errorf("empty lists did not clear: %+v", res.Token)
	}
	back, _ := e.svc.GetToken(ctx, e.camp, tok.ID)
	if len(back.Owners) != 0 || len(back.Bars) != 0 || len(back.Conditions) != 0 || back.Name != "Renamed" {
		t.Errorf("stored token = %+v", back)
	}
}

func TestTokenImage(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	img := e.image(e.camp, 40, 40)
	foreign := e.image(e.other, 41, 40)

	tok := e.token(scene.SceneID, protocol.TokenFields{ImageAssetID: json.RawMessage(`"` + img.ID + `"`)})
	if tok.Image == nil || tok.Image.AssetID != img.ID || tok.Image.URL != "/assets/"+img.ID {
		t.Fatalf("image = %+v", tok.Image)
	}
	if err := e.assets.Delete(ctx, img.ID); !errors.Is(err, asset.ErrInUse) {
		t.Errorf("deleting the image of a token = %v, want asset.ErrInUse", err)
	}

	for raw, want := range map[string]error{
		`"` + foreign.ID + `"`: ErrNotFound, // an asset of another campaign
		`"nope"`:               ErrNotFound,
		`5`:                    ErrInvalid,
	} {
		_, err := e.svc.UpdateToken(ctx, e.camp, protocol.TokenUpdate{
			TokenID: tok.ID, TokenFields: protocol.TokenFields{ImageAssetID: json.RawMessage(raw)},
		})
		if !errors.Is(err, want) {
			t.Errorf("image %s: err = %v, want %v", raw, err, want)
		}
	}

	res, err := e.svc.UpdateToken(ctx, e.camp, protocol.TokenUpdate{
		TokenID: tok.ID, TokenFields: protocol.TokenFields{ImageAssetID: json.RawMessage(`null`)},
	})
	if err != nil || res.Token.Image != nil {
		t.Fatalf("null image: %+v, %v", res.Token.Image, err)
	}
	if err := e.assets.Delete(ctx, img.ID); err != nil {
		t.Errorf("deleting a free asset: %v", err)
	}
}

func TestTokensOfAnotherCampaignAreNotFound(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	tok := e.token(scene.SceneID, protocol.TokenFields{})
	ctx := context.Background()

	if _, err := e.svc.CreateToken(ctx, e.other, protocol.TokenCreate{SceneID: scene.SceneID, TokenFields: protocol.TokenFields{X: fp(1), Y: fp(1)}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("create on a scene of another campaign: %v", err)
	}
	if _, err := e.svc.UpdateToken(ctx, e.other, protocol.TokenUpdate{TokenID: tok.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update: %v", err)
	}
	mayAll := func(protocol.Token, sceneState) error { return nil }
	if _, err := e.svc.MoveToken(ctx, e.other, protocol.TokenMove{TokenID: tok.ID, X: 1, Y: 1}, mayAll); !errors.Is(err, ErrNotFound) {
		t.Errorf("move: %v", err)
	}
	if _, err := e.svc.DeleteToken(ctx, e.other, tok.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete: %v", err)
	}
	if _, err := e.svc.GetToken(ctx, e.camp, tok.ID); err != nil {
		t.Errorf("the token must survive: %v", err)
	}
}

func TestMoveTokenChecksBoundsAndRight(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	tok := e.token(scene.SceneID, protocol.TokenFields{})
	mayAll := func(protocol.Token, sceneState) error { return nil }

	if _, err := e.svc.MoveToken(ctx, e.camp, protocol.TokenMove{TokenID: tok.ID, X: 2801, Y: 5}, mayAll); !errors.Is(err, ErrInvalid) {
		t.Errorf("x out of the scene: %v", err)
	}
	refuse := errors.New("refused")
	if _, err := e.svc.MoveToken(ctx, e.camp, protocol.TokenMove{TokenID: tok.ID, X: 5, Y: 5}, func(protocol.Token, sceneState) error { return refuse }); !errors.Is(err, refuse) {
		t.Errorf("refusal: %v", err)
	}
	res, err := e.svc.MoveToken(ctx, e.camp, protocol.TokenMove{TokenID: tok.ID, X: 7, Y: 9}, mayAll)
	if err != nil || res.Token.X != 7 || res.Token.Y != 9 {
		t.Fatalf("move = %+v, %v", res.Token, err)
	}
	back, _ := e.svc.GetToken(ctx, e.camp, tok.ID)
	if back.X != 7 || back.Y != 9 {
		t.Errorf("stored position = %v,%v", back.X, back.Y)
	}
}

func TestCheckDragWritesNothing(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	tok := e.token(scene.SceneID, protocol.TokenFields{})
	mayAll := func(protocol.Token, sceneState) error { return nil }

	res, ok, err := e.svc.CheckDrag(ctx, e.camp, protocol.TokenMove{TokenID: tok.ID, X: 500, Y: 500}, mayAll)
	if err != nil || !ok || res.Token.ID != tok.ID {
		t.Fatalf("CheckDrag = %+v, %t, %v", res, ok, err)
	}
	back, _ := e.svc.GetToken(ctx, e.camp, tok.ID)
	if back.X != 100 || back.Y != 100 {
		t.Errorf("a drag moved the stored token to %v,%v", back.X, back.Y)
	}
	for name, in := range map[string]protocol.TokenMove{
		"out of the scene": {TokenID: tok.ID, X: -1, Y: 0},
		"NaN":              {TokenID: tok.ID, X: math.NaN(), Y: 0},
		"unknown token":    {TokenID: "nope", X: 1, Y: 1},
	} {
		if _, ok, err := e.svc.CheckDrag(ctx, e.camp, in, mayAll); ok || err != nil {
			t.Errorf("%s: ok=%t err=%v, want a silent refusal", name, ok, err)
		}
	}
	refuse := func(protocol.Token, sceneState) error { return ErrNotFound }
	if _, ok, _ := e.svc.CheckDrag(ctx, e.camp, protocol.TokenMove{TokenID: tok.ID, X: 1, Y: 1}, refuse); ok {
		t.Error("a refused drag was accepted")
	}
}

func TestTokensListedByZThenID(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern")
	ctx := context.Background()
	a := e.token(scene.SceneID, protocol.TokenFields{Z: ip(2)})
	b := e.token(scene.SceneID, protocol.TokenFields{Z: ip(1)})
	c := e.token(scene.SceneID, protocol.TokenFields{Z: ip(2)})
	list, err := listTokens(ctx, e.db, scene.SceneID)
	if err != nil {
		t.Fatal(err)
	}
	first, second := a.ID, c.ID
	if second < first {
		first, second = second, first
	}
	want := []string{b.ID, first, second}
	if len(list) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(list), len(want))
	}
	for i, tok := range list {
		if tok.ID != want[i] {
			t.Errorf("order[%d] = %s, want %s", i, tok.ID, want[i])
		}
	}
	empty, err := listTokens(ctx, e.db, "nope")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("tokens of no scene = %#v, %v; want an empty non-nil list", empty, err)
	}
}

func TestDeleteTokenAndScene(t *testing.T) {
	e := newEnv(t)
	e.create("Tavern") // active
	other := e.create("Other")
	ctx := context.Background()
	tok := e.token(other.SceneID, protocol.TokenFields{})
	res, err := e.svc.DeleteToken(ctx, e.camp, tok.ID)
	if err != nil || res.Token.ID != tok.ID || res.State.Active {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	if _, err := e.svc.GetToken(ctx, e.camp, tok.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	// Deleting a scene takes its tokens along.
	t2 := e.token(other.SceneID, protocol.TokenFields{})
	if _, err := e.svc.DeleteScene(ctx, e.camp, other.SceneID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetToken(ctx, e.camp, t2.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("token of a deleted scene: %v", err)
	}
}

func TestPlayerCanSeeTokenImage(t *testing.T) {
	e := newEnv(t)
	scene := e.create("Tavern") // active
	inactiveScene := e.create("Other")
	ctx := context.Background()
	ann := e.player(e.camp, "ann", false)
	bob := e.player(e.camp, "bob", false)
	raw := func(a asset.Asset) json.RawMessage { return json.RawMessage(`"` + a.ID + `"`) }
	imgs := map[string]asset.Asset{}
	for i, name := range []string{"visible", "hidden", "gm", "inactive"} {
		imgs[name] = e.image(e.camp, 50+i, 50)
	}
	gm := "gm"
	e.token(scene.SceneID, protocol.TokenFields{ImageAssetID: raw(imgs["visible"])})
	e.token(scene.SceneID, protocol.TokenFields{ImageAssetID: raw(imgs["hidden"]), Hidden: bp(true), Owners: []string{ann}})
	e.token(scene.SceneID, protocol.TokenFields{ImageAssetID: raw(imgs["gm"]), Layer: &gm, Owners: []string{ann}})
	e.token(inactiveScene.SceneID, protocol.TokenFields{ImageAssetID: raw(imgs["inactive"])})

	for _, tt := range []struct {
		image, player string
		want          bool
	}{
		{"visible", bob, true},
		{"hidden", ann, true},
		{"hidden", bob, false},
		{"gm", ann, false},
		{"inactive", ann, false},
	} {
		got, err := e.svc.PlayerCanSeeTokenImage(ctx, e.camp, tt.player, imgs[tt.image].ID)
		if err != nil || got != tt.want {
			t.Errorf("%s for %s = %t, %v; want %t", tt.image, tt.player, got, err, tt.want)
		}
	}
}
