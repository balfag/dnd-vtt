package vtt

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/store"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

type env struct {
	t      *testing.T
	db     *sql.DB
	svc    *Service
	assets *asset.Service
	camp   string
	other  string // a second campaign
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	e := &env{t: t, db: st.DB()}
	e.svc = NewService(e.db, Options{Now: func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }})
	e.assets = asset.NewService(e.db, t.TempDir(), asset.Options{})
	e.camp = e.campaign("camp-1")
	e.other = e.campaign("camp-2")
	return e
}

func (e *env) campaign(id string) string {
	e.t.Helper()
	if _, err := e.db.Exec(`INSERT INTO campaign (id, name, created_at) VALUES (?, ?, 1)`, id, "Campaign "+id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) image(campaign string, w, h int) asset.Asset {
	e.t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		e.t.Fatal(err)
	}
	a, _, err := e.assets.Upload(context.Background(), campaign, "bg.png", "map", &b)
	if err != nil {
		e.t.Fatal(err)
	}
	return a
}

func (e *env) create(name string) CreateResult {
	e.t.Helper()
	res, err := e.svc.CreateScene(context.Background(), e.camp, protocol.SceneCreate{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func intp(n int) *int { return &n }

func strp(s string) *string { return &s }

func TestCreateSceneDefaults(t *testing.T) {
	e := newEnv(t)
	first := e.create("  Tavern  ")
	second := e.create("Crypt")

	if first.Activated == nil || first.Activated.ID != first.SceneID {
		t.Fatalf("the first scene must become active: %+v", first.Activated)
	}
	if second.Activated != nil {
		t.Errorf("the second scene must not change the active one: %+v", second.Activated)
	}
	v := first.Activated
	if v.Name != "Tavern" || v.Width != 2800 || v.Height != 2100 || v.Grid != DefaultGrid() || v.Background != nil {
		t.Errorf("view = %+v", v)
	}
	if v.Fog.Enabled || len(v.Fog.Ops) != 0 {
		t.Errorf("fog = %+v, want the default off with no ops", v.Fog)
	}
	if len(second.Scenes) != 2 || second.Scenes[0].Sort != 0 || second.Scenes[1].Sort != 1 || second.Scenes[1].Name != "Crypt" {
		t.Errorf("scenes = %+v, want sort 0 and 1 in order", second.Scenes)
	}
}

func TestSceneLimits(t *testing.T) {
	grid := func(mut func(*protocol.Grid)) *protocol.Grid {
		g := DefaultGrid()
		mut(&g)
		return &g
	}
	tests := []struct {
		name string
		in   protocol.SceneCreate
		ok   bool
	}{
		{"name of one rune", protocol.SceneCreate{Name: "x"}, true},
		{"name of 200 runes", protocol.SceneCreate{Name: strings.Repeat("ж", 200)}, true},
		{"empty name", protocol.SceneCreate{Name: ""}, false},
		{"blank name", protocol.SceneCreate{Name: "   "}, false},
		{"name of 201 runes", protocol.SceneCreate{Name: strings.Repeat("ж", 201)}, false},
		{"width 100", protocol.SceneCreate{Name: "a", Width: intp(100)}, true},
		{"width 99", protocol.SceneCreate{Name: "a", Width: intp(99)}, false},
		{"width 20000", protocol.SceneCreate{Name: "a", Width: intp(20000)}, true},
		{"width 20001", protocol.SceneCreate{Name: "a", Width: intp(20001)}, false},
		{"height 100", protocol.SceneCreate{Name: "a", Height: intp(100)}, true},
		{"height 99", protocol.SceneCreate{Name: "a", Height: intp(99)}, false},
		{"height 20001", protocol.SceneCreate{Name: "a", Height: intp(20001)}, false},
		{"hexPointy", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Type = "hexPointy" })}, true},
		{"hexFlat", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Type = "hexFlat" })}, true},
		{"unknown grid type", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Type = "octagon" })}, false},
		{"sizePx 10", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.SizePx = 10; g.OffsetX, g.OffsetY = 0, 0 })}, true},
		{"sizePx 9", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.SizePx = 9 })}, false},
		{"sizePx 500", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.SizePx = 500 })}, true},
		{"sizePx 501", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.SizePx = 501 })}, false},
		{"unitFt 1", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.UnitFt = 1 })}, true},
		{"unitFt 0", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.UnitFt = 0 })}, false},
		{"unitFt 100", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.UnitFt = 100 })}, true},
		{"unitFt 101", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.UnitFt = 101 })}, false},
		{"offset equal to sizePx", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.OffsetX, g.OffsetY = 70, 70 })}, true},
		{"offsetX over sizePx", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.OffsetX = 70.5 })}, false},
		{"negative offsetY", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.OffsetY = -0.5 })}, false},
		{"color in upper case", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Color = "#9AA8B3" })}, false},
		{"short color", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Color = "#fff" })}, false},
		{"color without a hash", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Color = "9aa8b3" })}, false},
		{"hidden grid", protocol.SceneCreate{Name: "a", Grid: grid(func(g *protocol.Grid) { g.Visible = false })}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			_, err := e.svc.CreateScene(context.Background(), e.camp, tt.in)
			if tt.ok && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if !tt.ok {
				if list, _ := listScenes(context.Background(), e.db, e.camp); len(list) != 0 {
					t.Errorf("a rejected scene left a row: %v", list)
				}
			}
		})
	}
}

func TestUpdateSceneLimitsAndPartialUpdate(t *testing.T) {
	e := newEnv(t)
	id := e.create("Tavern").SceneID
	ctx := context.Background()

	bad := []protocol.SceneUpdate{
		{SceneID: id, Name: strp("")},
		{SceneID: id, Name: strp(strings.Repeat("x", 201))},
		{SceneID: id, Width: intp(99)},
		{SceneID: id, Height: intp(20001)},
		{SceneID: id, Grid: &protocol.Grid{Type: "square", SizePx: 70, UnitFt: 5, Color: "#GGGGGG", Visible: true}},
	}
	for i, in := range bad {
		if _, err := e.svc.UpdateScene(ctx, e.camp, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("update %d: err = %v, want ErrInvalid", i, err)
		}
	}

	hex := DefaultGrid()
	hex.Type, hex.SizePx = "hexFlat", 90
	res, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, Name: strp(" Crypt "), Width: intp(4000), Grid: &hex})
	if err != nil {
		t.Fatal(err)
	}
	v := res.Scene
	if v.Name != "Crypt" || v.Width != 4000 || v.Height != 2100 || v.Grid != hex {
		t.Errorf("view = %+v, want the given fields changed and the height kept", v)
	}
	if !res.WasActive {
		t.Error("the only scene is the active one")
	}

	if _, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: "nope"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown scene: err = %v, want ErrNotFound", err)
	}
}

func TestSceneBackground(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.create("Tavern").SceneID
	big := e.image(e.camp, 300, 200)
	tiny := e.image(e.camp, 50, 40)
	foreign := e.image(e.other, 60, 60)

	set := func(raw string, w, h *int) (UpdateResult, error) {
		return e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, Width: w, Height: h, BackgroundAssetID: json.RawMessage(raw)})
	}

	if _, err := set(`"`+foreign.ID+`"`, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("a background of another campaign: err = %v, want ErrNotFound", err)
	}
	if _, err := set(`"no-such-asset"`, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown asset: err = %v, want ErrNotFound", err)
	}
	for _, raw := range []string{`123`, `""`, `{}`, `true`} {
		if _, err := set(raw, nil, nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("backgroundAssetId %s: err = %v, want ErrInvalid", raw, err)
		}
	}

	res, err := set(`"`+big.ID+`"`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Scene; v.Background == nil || v.Background.AssetID != big.ID || v.Background.URL != "/assets/"+big.ID {
		t.Fatalf("background = %+v", v.Background)
	}
	if v := res.Scene; v.Width != 300 || v.Height != 200 {
		t.Errorf("size = %dx%d, want the 300x200 of the image", v.Width, v.Height)
	}

	// A size given in the same command wins; the other one comes from the image.
	res, err = set(`"`+tiny.ID+`"`, intp(1000), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Scene; v.Width != 1000 || v.Height != 100 {
		t.Errorf("size = %dx%d, want 1000 from the command and 100 (50x40 raised to the minimum)", v.Width, v.Height)
	}

	// An absent field leaves the background; null removes it.
	res, err = e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, Name: strp("Renamed")})
	if err != nil || res.Scene.Background == nil || res.Scene.Background.AssetID != tiny.ID {
		t.Fatalf("a rename must keep the background: %+v, %v", res.Scene.Background, err)
	}
	res, err = set(`null`, nil, nil)
	if err != nil || res.Scene.Background != nil {
		t.Fatalf("null must remove the background: %+v, %v", res.Scene.Background, err)
	}
	if res.Scene.Width != 1000 {
		t.Errorf("removing the background changed the width to %d", res.Scene.Width)
	}
}

func TestBackgroundBlocksDeletingTheAsset(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := e.create("Tavern").SceneID
	a := e.image(e.camp, 300, 200)
	if _, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, BackgroundAssetID: json.RawMessage(`"` + a.ID + `"`)}); err != nil {
		t.Fatal(err)
	}
	if err := e.assets.Delete(ctx, a.ID); !errors.Is(err, asset.ErrInUse) {
		t.Fatalf("Delete of a background: err = %v, want ErrInUse", err)
	}
	if _, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, BackgroundAssetID: json.RawMessage(`null`)}); err != nil {
		t.Fatal(err)
	}
	if err := e.assets.Delete(ctx, a.ID); err != nil {
		t.Errorf("Delete after the background is removed: %v", err)
	}
}

func TestDeleteScene(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	active := e.create("Active").SceneID
	other := e.create("Other").SceneID

	if _, err := e.svc.DeleteScene(ctx, e.camp, active); !errors.Is(err, ErrConflict) {
		t.Fatalf("deleting the active scene: err = %v, want ErrConflict", err)
	}

	// A token and a drawing of the scene go with it.
	if _, err := e.db.Exec(`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES ('a1', 'anna', '$argon2id$x', 0, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO player (id, campaign_id, account_id, display_name, color, role, created_at) VALUES ('p1', ?, 'a1', 'Anna', '#e53935', 'player', 1)`, e.camp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO token (id, scene_id, layer, x, y, w, h, rotation, z, name, name_visibility, hidden, locked, owners, bars, conditions, created_at, updated_at)
		VALUES ('t1', ?, 'tokens', 1, 2, 1, 1, 0, 0, 'Orc', 'all', 0, 0, '[]', '[]', '[]', 1, 1)`, other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO drawing (id, scene_id, author_player_id, kind, data, color, created_at, updated_at)
		VALUES ('d1', ?, 'p1', 'rect', '{}', '#ff0000', 1, 1)`, other); err != nil {
		t.Fatal(err)
	}

	left, err := e.svc.DeleteScene(ctx, e.camp, other)
	if err != nil || len(left) != 1 || left[0].ID != active {
		t.Fatalf("DeleteScene = %+v, %v", left, err)
	}
	for _, table := range []string{"token", "drawing", "scene"} {
		var n int
		q := `SELECT COUNT(*) FROM ` + table + ` WHERE ` + map[string]string{"token": "scene_id", "drawing": "scene_id", "scene": "id"}[table] + ` = ?`
		if err := e.db.QueryRow(q, other).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s rows of the deleted scene = %d, %v; want 0", table, n, err)
		}
	}
}

func TestReorderScenes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, b, c := e.create("A").SceneID, e.create("B").SceneID, e.create("C").SceneID
	foreign, err := e.svc.CreateScene(ctx, e.other, protocol.SceneCreate{Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}

	for name, ids := range map[string][]string{
		"a missing one":    {a, b},
		"an extra one":     {a, b, c, "d"},
		"a repeated one":   {a, a, b},
		"a repeat of all":  {a, b, c, a},
		"a foreign scene":  {a, b, foreign.SceneID},
		"an empty list":    {},
		"only the foreign": {foreign.SceneID},
	} {
		if _, err := e.svc.ReorderScenes(ctx, e.camp, ids); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if list, _ := listScenes(ctx, e.db, e.camp); list[0].ID != a || list[1].ID != b || list[2].ID != c {
		t.Errorf("a rejected reorder changed the order: %+v", list)
	}

	list, err := e.svc.ReorderScenes(ctx, e.camp, []string{c, a, b})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{c, a, b} {
		if list[i].ID != want || list[i].Sort != i {
			t.Errorf("list[%d] = %+v, want %s with sort %d", i, list[i], want, i)
		}
	}
}

func TestScenesOfAnotherCampaignAreNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	foreign, err := e.svc.CreateScene(ctx, e.other, protocol.SceneCreate{Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	id := foreign.SceneID

	if _, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, Name: strp("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update: err = %v", err)
	}
	if _, err := e.svc.DeleteScene(ctx, e.camp, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete: err = %v", err)
	}
	if _, err := e.svc.ActivateScene(ctx, e.camp, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("activate: err = %v", err)
	}
	if _, err := e.svc.ViewScene(ctx, e.camp, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("view: err = %v", err)
	}
	if _, err := e.svc.CreateScene(ctx, "no-campaign", protocol.SceneCreate{Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("create in an unknown campaign: err = %v", err)
	}
}

func TestActivateAndBackgroundOfActiveScene(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first := e.create("First").SceneID
	second := e.create("Second").SceneID
	bg1, bg2 := e.image(e.camp, 300, 200), e.image(e.camp, 301, 200)
	for id, bg := range map[string]string{first: bg1.ID, second: bg2.ID} {
		if _, err := e.svc.UpdateScene(ctx, e.camp, protocol.SceneUpdate{SceneID: id, BackgroundAssetID: json.RawMessage(`"` + bg + `"`)}); err != nil {
			t.Fatal(err)
		}
	}

	check := func(asset string, want bool) {
		t.Helper()
		got, err := e.svc.BackgroundOfActiveScene(ctx, e.camp, asset)
		if err != nil || got != want {
			t.Errorf("BackgroundOfActiveScene(%s) = %v, %v; want %v", asset, got, err, want)
		}
	}
	check(bg1.ID, true)
	check(bg2.ID, false)

	if _, err := e.svc.ActivateScene(ctx, e.camp, second); err != nil {
		t.Fatal(err)
	}
	check(bg1.ID, false)
	check(bg2.ID, true)

	if got, _ := e.svc.BackgroundOfActiveScene(ctx, e.other, bg2.ID); got {
		t.Error("the background counts for another campaign")
	}
}

func TestFillSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	empty := protocol.SnapshotTable{Scenes: []protocol.SceneListItem{}}

	t0 := empty
	if err := e.svc.FillSnapshot(ctx, ws.Viewer{IsGM: true}, e.camp, &t0); err != nil {
		t.Fatal(err)
	}
	if t0.ActiveSceneID != nil || t0.Scene != nil || t0.Scenes == nil || len(t0.Scenes) != 0 {
		t.Errorf("no scenes: %+v, want null, null and an empty array", t0)
	}

	first := e.create("First")
	second := e.create("Second")

	gm := empty
	if err := e.svc.FillSnapshot(ctx, ws.Viewer{IsGM: true}, e.camp, &gm); err != nil {
		t.Fatal(err)
	}
	if gm.ActiveSceneID == nil || *gm.ActiveSceneID != first.SceneID || gm.Scene == nil || gm.Scene.ID != first.SceneID {
		t.Errorf("GM table = %+v, want the first scene active", gm)
	}
	if len(gm.Scenes) != 2 || gm.Scenes[0].ID != first.SceneID || gm.Scenes[1].ID != second.SceneID {
		t.Errorf("GM scenes = %+v, want both in order", gm.Scenes)
	}

	player := empty
	if err := e.svc.FillSnapshot(ctx, ws.Viewer{PlayerID: "p1"}, e.camp, &player); err != nil {
		t.Fatal(err)
	}
	if player.Scenes == nil || len(player.Scenes) != 0 {
		t.Errorf("player scenes = %#v, want an empty non-nil array", player.Scenes)
	}
	if player.Scene == nil || player.Scene.ID != first.SceneID || player.ActiveSceneID == nil {
		t.Errorf("player table = %+v, want the active scene", player)
	}
}

func TestDeletingTheActiveSceneRowClearsTheCampaign(t *testing.T) {
	e := newEnv(t)
	id := e.create("Only").SceneID
	if _, err := e.db.Exec(`DELETE FROM scene WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	active, err := activeSceneID(context.Background(), e.db, e.camp)
	if err != nil || active != "" {
		t.Errorf("active scene = %q, %v; want none (ON DELETE SET NULL)", active, err)
	}
}
