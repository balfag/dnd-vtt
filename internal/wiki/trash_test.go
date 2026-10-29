package wiki

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/asset"
)

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) ftsHits(term string) int {
	return e.count(`SELECT COUNT(*) FROM wiki_fts WHERE wiki_fts MATCH ?`, term)
}

func TestTrashList(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "A")
	b := e.create(a.ID, "B")
	e.create(b.ID, "C")
	d := e.create("", "D")

	e.clock = e.clock.Add(time.Hour)
	deletedA := e.clock
	if err := e.svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	items, err := e.svc.Trash(ctx, e.camp)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != a.ID || items[0].Title != "A" || items[0].Kind != KindPage || items[0].PageCount != 3 {
		t.Fatalf("items = %+v", items)
	}
	if !items[0].DeletedAt.Equal(deletedA) || !items[0].PurgeAt.Equal(deletedA.Add(TrashTTL)) {
		t.Errorf("times = %v, %v", items[0].DeletedAt, items[0].PurgeAt)
	}

	e.clock = e.clock.Add(time.Hour)
	if err := e.svc.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	items, err = e.svc.Trash(ctx, e.camp)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != d.ID || items[1].ID != a.ID || items[0].PageCount != 1 {
		t.Errorf("items = %+v", items)
	}
	if _, err := e.svc.Trash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign = %v", err)
	}
}

func TestRestore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	parent := e.create("", "Parent")
	e.create(parent.ID, "Old")
	child := e.create(parent.ID, "Child")
	grand := e.create(child.ID, "Grand")
	e.mustSave(grand.ID, 1, "Grand", para(text("moonlit harbor")))
	e.create(parent.ID, "Last")

	e.clock = e.clock.Add(time.Hour)
	if err := e.svc.Delete(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	if e.ftsHits("moonlit") != 0 {
		t.Fatal("search row stays after delete")
	}
	if err := e.svc.Restore(ctx, grand.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Restore of a non-root = %v", err)
	}
	if err := e.svc.Restore(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Restore of an unknown id = %v", err)
	}
	if err := e.svc.Restore(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Get(ctx, child.ID)
	if err != nil || got.ParentID != parent.ID {
		t.Fatalf("restored = %+v, %v", got, err)
	}
	if want := []string{"Old=0", "Last=1", "Child=2"}; !equalSet(e.positions(parent.ID), want) {
		t.Errorf("positions = %v", e.positions(parent.ID))
	}
	g, err := e.svc.Get(ctx, grand.ID)
	if err != nil || g.ParentID != child.ID {
		t.Errorf("grandchild = %+v, %v", g, err)
	}
	if e.ftsHits("moonlit") != 1 {
		t.Error("search row is not back")
	}
	if items, _ := e.svc.Trash(ctx, e.camp); len(items) != 0 {
		t.Errorf("trash = %+v", items)
	}

	// The parent is in the trash: the root goes to the top level.
	e.clock = e.clock.Add(time.Hour)
	if err := e.svc.Delete(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	e.create("", "Top")
	if err := e.svc.Restore(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	got, err = e.svc.Get(ctx, child.ID)
	if err != nil || got.ParentID != "" {
		t.Fatalf("restored = %+v, %v", got, err)
	}
	if want := []string{"Top=0", "Child=1"}; !equalSet(e.positions(""), want) {
		t.Errorf("root positions = %v", e.positions(""))
	}
}

func equalSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestPurge(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	keep := e.create("", "Keep")
	e.mustSave(keep.ID, 1, "Keep", para(text("lantern")))
	a := e.create("", "A")
	b := e.create(a.ID, "B")
	img := e.asset(e.camp)
	e.mustSave(b.ID, 1, "B", imageDoc(img))
	target := e.create("", "Target")
	e.mustSave(a.ID, 1, "A", para(text("dragon"), mention("page", target.ID)))
	if _, err := e.svc.SetTags(ctx, b.ID, []string{"x"}); err != nil {
		t.Fatal(err)
	}

	assets := asset.NewService(e.db, t.TempDir(), asset.Options{})
	if err := assets.Delete(ctx, img); !errors.Is(err, asset.ErrInUse) {
		t.Fatalf("Delete of a used asset = %v", err)
	}
	if err := e.svc.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Purge(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Purge of a non-root = %v", err)
	}
	if err := e.svc.Purge(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM wiki_page WHERE id IN (?1, ?2)`,
		`SELECT COUNT(*) FROM wiki_link WHERE page_id IN (?1, ?2)`,
		`SELECT COUNT(*) FROM wiki_page_asset WHERE page_id IN (?1, ?2)`,
		`SELECT COUNT(*) FROM wiki_page_tag WHERE page_id IN (?1, ?2)`,
		`SELECT COUNT(*) FROM wiki_fts WHERE page_id IN (?1, ?2)`,
	} {
		if n := e.count(q, a.ID, b.ID); n != 0 {
			t.Errorf("%s = %d", q, n)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_page WHERE id = ?`, keep.ID); n != 1 {
		t.Error("other page is gone")
	}
	if e.ftsHits("lantern") != 1 {
		t.Error("search row of another page is gone")
	}
	if err := assets.Delete(ctx, img); err != nil {
		t.Errorf("Delete of the freed asset = %v", err)
	}
}

func TestEmptyTrash(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "A")
	e.create(a.ID, "B")
	c := e.create("", "C")
	live := e.create("", "Live")
	other := &env{t: t, db: e.db, svc: e.svc, clock: e.clock}
	other.camp = e.campaign("Other")
	o := other.create("", "O")

	for _, pid := range []string{a.ID, c.ID, o.ID} {
		if err := e.svc.Delete(ctx, pid); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.svc.EmptyTrash(ctx, e.camp); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_page WHERE campaign_id = ?`, e.camp); n != 1 {
		t.Errorf("pages left = %d", n)
	}
	if _, err := e.svc.Get(ctx, live.ID); err != nil {
		t.Error(err)
	}
	if items, _ := e.svc.Trash(ctx, other.camp); len(items) != 1 {
		t.Errorf("other trash = %+v", items)
	}
	if err := e.svc.EmptyTrash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign = %v", err)
	}
}

func TestPurgeExpired(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old := e.create("", "Old")
	e.create(old.ID, "OldChild")
	fresh := e.create("", "Fresh")

	start := e.clock
	e.clock = start.Add(-31 * 24 * time.Hour)
	if err := e.svc.Delete(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	e.clock = start.Add(-29 * 24 * time.Hour)
	if err := e.svc.Delete(ctx, fresh.ID); err != nil {
		t.Fatal(err)
	}
	e.clock = start
	n, err := e.svc.PurgeExpired(ctx)
	if err != nil || n != 1 {
		t.Fatalf("PurgeExpired = %d, %v", n, err)
	}
	items, _ := e.svc.Trash(ctx, e.camp)
	if len(items) != 1 || items[0].ID != fresh.ID {
		t.Errorf("trash = %+v", items)
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_page WHERE title LIKE 'Old%'`); n != 0 {
		t.Errorf("old pages left = %d", n)
	}
	if n, err := e.svc.PurgeExpired(ctx); err != nil || n != 0 {
		t.Errorf("second PurgeExpired = %d, %v", n, err)
	}
}
