package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/id"
	"github.com/balfag/dnd-vtt/internal/store"
	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

type env struct {
	t     *testing.T
	db    *sql.DB
	svc   *Service
	camp  string
	clock time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	e := &env{t: t, db: st.DB(), clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	e.svc = NewService(e.db, Options{Now: func() time.Time { return e.clock }})
	e.camp = e.campaign("Camp")
	return e
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(query, args...); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func (e *env) campaign(name string) string {
	e.t.Helper()
	cid := "camp-" + name
	e.exec(`INSERT INTO campaign (id, name, created_at) VALUES (?, ?, 1)`, cid, name)
	return cid
}

func (e *env) asset(campaign string) string {
	e.t.Helper()
	aid := id.New()
	e.exec(`INSERT INTO asset (id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at)
		VALUES (?, ?, ?, 'image/png', 1, 1, 1, 'a.png', 'other', 1)`, aid, campaign, strings.Repeat("a", 60)+aid[:4])
	return aid
}

func (e *env) scene(campaign, name string) string {
	e.t.Helper()
	sid := id.New()
	e.exec(`INSERT INTO scene (id, campaign_id, name, sort, width, height, grid, created_at, updated_at)
		VALUES (?, ?, ?, 0, 1000, 1000, '{}', 1, 1)`, sid, campaign, name)
	return sid
}

func (e *env) token(scene, name string) string {
	e.t.Helper()
	tid := id.New()
	e.exec(`INSERT INTO token (id, scene_id, layer, x, y, w, h, rotation, z, name, name_visibility, hidden, locked, owners, bars, conditions, created_at, updated_at)
		VALUES (?, ?, 'tokens', 0, 0, 1, 1, 0, 0, ?, 'all', 0, 0, '[]', '[]', '[]', 1, 1)`, tid, scene, name)
	return tid
}

func (e *env) create(parent, title string) Page {
	e.t.Helper()
	p, err := e.svc.Create(context.Background(), e.camp, NewPage{ParentID: parent, Title: title})
	if err != nil {
		e.t.Fatalf("create %q: %v", title, err)
	}
	return p
}

func (e *env) database() string {
	e.t.Helper()
	did := id.New()
	e.exec(`INSERT INTO wiki_page (id, campaign_id, kind, title, position, content, content_text, version, created_at, updated_at)
		VALUES (?, ?, 'database', 'DB', 0, ?, '', 1, 1, 1)`, did, e.camp, string(doc.Empty()))
	return did
}

func (e *env) tree() []PageNode {
	e.t.Helper()
	nodes, err := e.svc.Tree(context.Background(), e.camp)
	if err != nil {
		e.t.Fatal(err)
	}
	return nodes
}

func (e *env) positions(parent string) []string {
	e.t.Helper()
	var out []string
	for _, n := range e.tree() {
		if n.ParentID == parent {
			out = append(out, fmt.Sprintf("%s=%d", n.Title, n.Position))
		}
	}
	return out
}

func para(inline ...string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[` + strings.Join(inline, ",") + `]}]}`
}

func text(s string) string { return `{"type":"text","text":` + fmt.Sprintf("%q", s) + `}` }

func mention(kind, mid string) string {
	return `{"type":"mention","attrs":{"kind":"` + kind + `","id":"` + mid + `"}}`
}

func imageDoc(assetID string) string {
	return `{"type":"doc","content":[{"type":"image","attrs":{"assetId":"` + assetID + `"}}]}`
}

func (e *env) save(pageID string, version int, title, content string) (Saved, error) {
	e.t.Helper()
	return e.svc.Save(context.Background(), pageID, SavePage{Version: version, Title: title, Content: json.RawMessage(content)})
}

func (e *env) mustSave(pageID string, version int, title, content string) Saved {
	e.t.Helper()
	s, err := e.save(pageID, version, title, content)
	if err != nil {
		e.t.Fatalf("save: %v", err)
	}
	return s
}

func TestCreateAndTree(t *testing.T) {
	e := newEnv(t)
	a := e.create("", "A")
	b := e.create("", "B")
	c := e.create(a.ID, "C")

	if a.Version != 1 || a.Kind != KindPage || a.IsRecord || string(a.Props) != "{}" || a.Tags == nil || a.Path == nil || a.Refs == nil {
		t.Errorf("created page = %+v", a)
	}
	if string(a.Content) != string(doc.Empty()) {
		t.Errorf("content = %s", a.Content)
	}
	if !a.CreatedAt.Equal(e.clock) {
		t.Errorf("createdAt = %v", a.CreatedAt)
	}

	got := e.tree()
	want := []PageNode{
		{ID: a.ID, Kind: KindPage, Title: "A", Position: 0},
		{ID: b.ID, Kind: KindPage, Title: "B", Position: 1},
		{ID: c.ID, ParentID: a.ID, Kind: KindPage, Title: "C", Position: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("tree = %+v, want %+v", got, want)
	}

	g, err := e.svc.Get(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Path) != 1 || g.Path[0] != (PageRef{ID: a.ID, Title: "A", Kind: KindPage}) || g.ParentID != a.ID {
		t.Errorf("path = %+v parent = %q", g.Path, g.ParentID)
	}
	if _, err := e.svc.Tree(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Tree of unknown campaign = %v", err)
	}
	if _, err := e.svc.Create(context.Background(), "nope", NewPage{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Create in unknown campaign = %v", err)
	}
	if _, err := e.svc.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get unknown = %v", err)
	}
}

func TestCreateInvalid(t *testing.T) {
	e := newEnv(t)
	other := e.campaign("Other")
	foreign, err := e.svc.Create(context.Background(), other, NewPage{Title: "F"})
	if err != nil {
		t.Fatal(err)
	}
	trashed := e.create("", "T")
	if err := e.svc.Delete(context.Background(), trashed.ID); err != nil {
		t.Fatal(err)
	}
	db := e.database()
	record := id.New()
	e.exec(`INSERT INTO wiki_page (id, campaign_id, parent_id, kind, title, position, content, content_text, version, created_at, updated_at)
		VALUES (?, ?, ?, 'page', 'R', 0, ?, '', 1, 1, 1)`, record, e.camp, db, string(doc.Empty()))
	foreignAsset := e.asset(other)

	tests := []struct {
		name string
		p    NewPage
		docs bool
	}{
		{"parent of another campaign", NewPage{ParentID: foreign.ID}, false},
		{"parent in the trash", NewPage{ParentID: trashed.ID}, false},
		{"unknown parent", NewPage{ParentID: id.New()}, false},
		{"database in a database", NewPage{ParentID: db, Kind: KindDatabase}, false},
		{"record parent", NewPage{ParentID: record}, false},
		{"title too long", NewPage{Title: strings.Repeat("я", MaxTitle+1)}, false},
		{"bad icon", NewPage{Icon: "123456789"}, false},
		{"bad document", NewPage{Content: json.RawMessage(`{"type":"nope"}`)}, true},
		{"foreign asset", NewPage{Content: json.RawMessage(imageDoc(foreignAsset))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.svc.Create(context.Background(), e.camp, tt.p)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if tt.docs && !errors.Is(err, doc.ErrInvalid) {
				t.Errorf("err = %v, want doc.ErrInvalid too", err)
			}
		})
	}

	if _, err := e.svc.Create(context.Background(), e.camp, NewPage{Title: strings.Repeat("я", MaxTitle)}); err != nil {
		t.Errorf("title of %d runes: %v", MaxTitle, err)
	}
}

func TestCreateDepth(t *testing.T) {
	e := newEnv(t)
	parent := ""
	for i := 1; i <= MaxDepth; i++ {
		parent = e.create(parent, fmt.Sprintf("L%d", i)).ID
	}
	_, err := e.svc.Create(context.Background(), e.camp, NewPage{ParentID: parent, Title: "too deep"})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("depth %d: %v, want ErrInvalid", MaxDepth+1, err)
	}
}

func TestCreatePageLimit(t *testing.T) {
	e := newEnv(t)
	e.exec(`WITH RECURSIVE n (i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO wiki_page (id, campaign_id, kind, title, position, content, content_text, version, created_at, updated_at)
		SELECT 'fill-' || i, ?, 'page', '', i, ?, '', 1, 1, 1 FROM n`, MaxPages, e.camp, string(doc.Empty()))
	_, err := e.svc.Create(context.Background(), e.camp, NewPage{})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestSave(t *testing.T) {
	e := newEnv(t)
	p := e.create("", "A")
	e.clock = e.clock.Add(time.Minute)

	s := e.mustSave(p.ID, 1, "A2", para(text("hi")))
	if s.Version != 2 || !s.UpdatedAt.Equal(e.clock) || s.Refs == nil {
		t.Errorf("saved = %+v", s)
	}
	if _, err := e.save(p.ID, 1, "A3", para()); !errors.Is(err, ErrConflict) {
		t.Errorf("stale save = %v, want ErrConflict", err)
	}
	forced, err := e.svc.Save(context.Background(), p.ID, SavePage{Version: 1, Title: "A3", Content: json.RawMessage(para()), Force: true})
	if err != nil || forced.Version != 3 {
		t.Errorf("forced save = %+v, %v", forced, err)
	}
	g, _ := e.svc.Get(context.Background(), p.ID)
	if g.Title != "A3" || g.Version != 3 {
		t.Errorf("page = %+v", g)
	}

	other := e.campaign("Other")
	_, err = e.svc.Save(context.Background(), p.ID, SavePage{Version: 3, CoverAssetID: e.asset(other), Content: json.RawMessage(para())})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("foreign cover = %v, want ErrInvalid", err)
	}
	cover := e.asset(e.camp)
	if _, err := e.svc.Save(context.Background(), p.ID, SavePage{Version: 3, CoverAssetID: cover, Content: json.RawMessage(para())}); err != nil {
		t.Fatal(err)
	}
	if g, _ := e.svc.Get(context.Background(), p.ID); g.CoverAssetID != cover {
		t.Errorf("cover = %q", g.CoverAssetID)
	}
	if _, err := e.svc.Save(context.Background(), p.ID, SavePage{Version: 4, Content: nil}); !errors.Is(err, ErrInvalid) {
		t.Errorf("save without content = %v, want ErrInvalid", err)
	}
	if _, err := e.save("nope", 1, "", para()); !errors.Is(err, ErrNotFound) {
		t.Errorf("save unknown = %v", err)
	}

	d := e.database()
	if _, err := e.save(d, 1, "DB", para(text("x"))); !errors.Is(err, ErrInvalid) {
		t.Errorf("database with content = %v, want ErrInvalid", err)
	}
	if _, err := e.save(d, 1, "DB", string(doc.Empty())); err != nil {
		t.Errorf("database with the empty document: %v", err)
	}
}

func TestIndexing(t *testing.T) {
	e := newEnv(t)
	p := e.create("", "Inn")
	target := e.create("", "Target")
	asset1 := e.asset(e.camp)

	content := `{"type":"doc","content":[
		{"type":"paragraph","content":[` + text("Таверна «Пони»") + `,` + mention("page", target.ID) + `]},
		{"type":"image","attrs":{"assetId":"` + asset1 + `"}}]}`
	e.mustSave(p.ID, 1, "Inn", content)

	var contentText string
	if err := e.db.QueryRow(`SELECT content_text FROM wiki_page WHERE id = ?`, p.ID).Scan(&contentText); err != nil || !strings.Contains(contentText, "Таверна") {
		t.Errorf("content_text = %q, %v", contentText, err)
	}
	count := func(query string, args ...any) int {
		var n int
		if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM wiki_fts WHERE wiki_fts MATCH 'таверн*' AND page_id = ?`, p.ID); n != 1 {
		t.Errorf("fts matches = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM wiki_link WHERE page_id = ? AND target_kind = 'page' AND target_id = ? AND source = 'mention'`, p.ID, target.ID); n != 1 {
		t.Errorf("links = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM wiki_page_asset WHERE page_id = ? AND asset_id = ?`, p.ID, asset1); n != 1 {
		t.Errorf("page assets = %d", n)
	}

	assets := asset.NewService(e.db, t.TempDir(), asset.Options{})
	if err := assets.Delete(context.Background(), asset1); !errors.Is(err, asset.ErrInUse) {
		t.Errorf("delete used asset = %v, want ErrInUse", err)
	}

	e.mustSave(p.ID, 2, "Inn", para(text("plain")))
	if n := count(`SELECT COUNT(*) FROM wiki_link WHERE page_id = ?`, p.ID); n != 0 {
		t.Errorf("links after save = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM wiki_page_asset WHERE page_id = ?`, p.ID); n != 0 {
		t.Errorf("page assets after save = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM wiki_fts WHERE page_id = ?`, p.ID); n != 1 {
		t.Errorf("fts rows = %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM wiki_fts WHERE wiki_fts MATCH 'таверн*'`); n != 0 {
		t.Errorf("stale fts matches = %d", n)
	}
	if err := assets.Delete(context.Background(), asset1); err != nil {
		t.Errorf("delete unused asset: %v", err)
	}
}

func TestRefs(t *testing.T) {
	e := newEnv(t)
	p := e.create("", "P")
	live := e.create("", "Old name")
	trashed := e.create("", "Gone")
	scene := e.scene(e.camp, "Crypt")
	token := e.token(scene, "Goblin")
	otherCamp := e.campaign("Other")
	foreignScene := e.scene(otherCamp, "Foreign")
	missing := id.New()

	if err := e.svc.Delete(context.Background(), trashed.ID); err != nil {
		t.Fatal(err)
	}
	e.mustSave(live.ID, 1, "New name", para())

	s := e.mustSave(p.ID, 1, "P", para(
		mention("page", live.ID), mention("page", trashed.ID), mention("scene", scene),
		mention("token", token), mention("page", missing), mention("scene", foreignScene)))

	want := map[string]RefInfo{
		"page:" + live.ID:       {Label: "New name", Exists: true},
		"page:" + trashed.ID:    {},
		"scene:" + scene:        {Label: "Crypt", Exists: true},
		"token:" + token:        {Label: "Goblin", Exists: true},
		"page:" + missing:       {},
		"scene:" + foreignScene: {},
	}
	if fmt.Sprint(s.Refs) != fmt.Sprint(want) {
		t.Errorf("saved refs = %v\nwant %v", s.Refs, want)
	}
	g, err := e.svc.Get(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(g.Refs) != fmt.Sprint(want) {
		t.Errorf("page refs = %v", g.Refs)
	}
}

func TestMove(t *testing.T) {
	e := newEnv(t)
	a := e.create("", "A")
	b := e.create("", "B")
	c := e.create("", "C")
	x := e.create(a.ID, "X")
	y := e.create(a.ID, "Y")
	_ = y
	ctx := context.Background()

	if err := e.svc.Move(ctx, c.ID, "", 0); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(e.positions("")); got != "[C=0 A=1 B=2]" {
		t.Errorf("root = %s", got)
	}
	if err := e.svc.Move(ctx, c.ID, "", 99); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(e.positions("")); got != "[A=0 B=1 C=2]" {
		t.Errorf("root after move to the end = %s", got)
	}

	if err := e.svc.Move(ctx, x.ID, b.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(e.positions(a.ID)); got != "[Y=0]" {
		t.Errorf("old siblings = %s", got)
	}
	if got := fmt.Sprint(e.positions(b.ID)); got != "[X=0]" {
		t.Errorf("new siblings = %s", got)
	}
	if err := e.svc.Move(ctx, x.ID, "", 1); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(e.positions("")); got != "[A=0 X=1 B=2 C=3]" {
		t.Errorf("root = %s", got)
	}

	if err := e.svc.Move(ctx, a.ID, a.ID, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("into itself = %v", err)
	}
	if err := e.svc.Move(ctx, a.ID, y.ID, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("into descendant = %v", err)
	}
	if err := e.svc.Move(ctx, a.ID, "", -1); !errors.Is(err, ErrInvalid) {
		t.Errorf("negative index = %v", err)
	}
	if err := e.svc.Move(ctx, "nope", "", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown page = %v", err)
	}
	if err := e.svc.Move(ctx, a.ID, id.New(), 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown parent = %v", err)
	}
	db := e.database()
	if err := e.svc.Move(ctx, a.ID, db, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("into a database = %v", err)
	}
	record := id.New()
	e.exec(`INSERT INTO wiki_page (id, campaign_id, parent_id, kind, title, position, content, content_text, version, created_at, updated_at)
		VALUES (?, ?, ?, 'page', 'R', 0, ?, '', 1, 1, 1)`, record, e.camp, db, string(doc.Empty()))
	if err := e.svc.Move(ctx, record, "", 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("moving a record = %v", err)
	}
}

func TestMoveDepth(t *testing.T) {
	e := newEnv(t)
	chain := ""
	for i := 1; i <= MaxDepth-1; i++ { // depth 19
		chain = e.create(chain, fmt.Sprintf("L%d", i)).ID
	}
	sub := e.create("", "S")
	e.create(sub.ID, "S1") // the subtree S has 2 levels
	ctx := context.Background()

	if err := e.svc.Move(ctx, sub.ID, chain, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("subtree to depth %d = %v, want ErrInvalid", MaxDepth+1, err)
	}
	leaf := e.create("", "Leaf")
	if err := e.svc.Move(ctx, leaf.ID, chain, 0); err != nil {
		t.Errorf("leaf to depth %d: %v", MaxDepth, err)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	before := e.create("", "Before")
	a := e.create("", "A")
	after := e.create("", "After")
	b := e.create(a.ID, "B")
	c := e.create(b.ID, "C")
	e.mustSave(c.ID, 1, "C", para(text("tavern")))

	e.clock = e.clock.Add(time.Hour)
	if err := e.svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []string{a.ID, b.ID, c.ID} {
		var deleted int64
		var root string
		if err := e.db.QueryRow(`SELECT deleted_at, trash_root_id FROM wiki_page WHERE id = ?`, pid).Scan(&deleted, &root); err != nil {
			t.Fatal(err)
		}
		if deleted != e.clock.UnixMilli() || root != a.ID {
			t.Errorf("page %s: deleted_at = %d, trash_root_id = %q", pid, deleted, root)
		}
		if _, err := e.svc.Get(ctx, pid); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get %s = %v", pid, err)
		}
		var n int
		_ = e.db.QueryRow(`SELECT COUNT(*) FROM wiki_fts WHERE page_id = ?`, pid).Scan(&n)
		if n != 0 {
			t.Errorf("fts rows of %s = %d", pid, n)
		}
	}
	if got := e.tree(); len(got) != 2 || got[0].ID != before.ID || got[1].ID != after.ID || got[1].Position != 1 {
		t.Errorf("tree = %+v", got)
	}
	if err := e.svc.Delete(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v", err)
	}
	if _, err := e.save(a.ID, 1, "", para()); !errors.Is(err, ErrNotFound) {
		t.Errorf("save of a trashed page = %v", err)
	}
}
