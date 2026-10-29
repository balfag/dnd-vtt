package wiki

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func (e *env) mentions(q string) []MentionHit {
	e.t.Helper()
	hits, err := e.svc.MentionSearch(context.Background(), e.camp, q)
	if err != nil {
		e.t.Fatalf("mention search %q: %v", q, err)
	}
	return hits
}

func TestMentionSearchKinds(t *testing.T) {
	e := newEnv(t)
	parent := e.create("", "Города")
	page := e.create(parent.ID, "Гавань")
	scene := e.scene(e.camp, "Гавань ночью")
	tok := e.token(scene, "Грифон")
	e.token(scene, "")
	other := e.campaign("Other")
	e.scene(other, "Гавань чужая")
	if _, err := e.svc.Create(context.Background(), other, NewPage{Title: "Гавань чужая"}); err != nil {
		t.Fatal(err)
	}

	got := e.mentions("ГАВАН")
	want := []MentionHit{
		{Kind: "page", ID: page.ID, Label: "Гавань", Detail: "Города"},
		{Kind: "scene", ID: scene, Label: "Гавань ночью", Detail: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %+v", got)
	}
	got = e.mentions("гриф")
	if want := []MentionHit{{Kind: "token", ID: tok, Label: "Грифон", Detail: "Гавань ночью"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("token hits = %+v", got)
	}
}

func TestMentionSearchOrder(t *testing.T) {
	e := newEnv(t)
	e.create("", "Дом дракона")
	e.create("", "Дракон")
	scene := e.scene(e.camp, "дракон")
	e.create("", "Аргон драконов")

	var labels []string
	for _, h := range e.mentions("дракон") {
		labels = append(labels, h.Kind+":"+h.Label)
	}
	// Prefix first (by name, then kind), then the rest by name.
	want := []string{"page:Дракон", "scene:дракон", "page:Аргон драконов", "page:Дом дракона"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("order = %v", labels)
	}
	_ = scene
}

func TestMentionSearchLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 25; i++ {
		e.create("", fmt.Sprintf("Эльф %02d", i))
	}
	if got := e.mentions("эльф"); len(got) != 20 {
		t.Errorf("hits = %d", len(got))
	}
}

func TestMentionSearchEmptyQuery(t *testing.T) {
	e := newEnv(t)
	e.scene(e.camp, "Сцена")
	for i := 0; i < 12; i++ {
		e.clock = e.clock.Add(time.Minute)
		e.create("", fmt.Sprintf("Стр %02d", i))
	}
	trashed := e.create("", "Корзина")
	if err := e.svc.Delete(context.Background(), trashed.ID); err != nil {
		t.Fatal(err)
	}

	got := e.mentions("")
	if len(got) != 10 {
		t.Fatalf("hits = %d", len(got))
	}
	for i, h := range got {
		if h.Kind != "page" || h.Label != fmt.Sprintf("Стр %02d", 11-i) {
			t.Errorf("hit %d = %+v", i, h)
		}
	}
}

func TestMentionSearchRecordDetail(t *testing.T) {
	e := newEnv(t)
	db := e.database()
	rec := e.record(db, "Бард")
	got := e.mentions("бард")
	if want := []MentionHit{{Kind: "page", ID: rec, Label: "Бард", Detail: "DB"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %+v", got)
	}
}

func TestMentionSearchInvalid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.MentionSearch(ctx, e.camp, strings.Repeat("я", 101)); !errors.Is(err, ErrInvalid) {
		t.Errorf("too long: %v", err)
	}
	if _, err := e.svc.MentionSearch(ctx, e.camp, strings.Repeat("я", 100)); err != nil {
		t.Errorf("100 characters: %v", err)
	}
	if _, err := e.svc.MentionSearch(ctx, "nope", "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
}

func TestBacklinks(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	target := e.create("", "Цель")
	src := e.create("", "Источник")
	e.create("", "Никто")

	if got, err := e.svc.Backlinks(ctx, target.ID); err != nil || len(got) != 0 || got == nil {
		t.Fatalf("no links: %+v, %v", got, err)
	}
	s := e.mustSave(src.ID, src.Version, "Источник", para(mention("page", target.ID)))
	got, err := e.svc.Backlinks(ctx, target.ID)
	if err != nil || len(got) != 1 || got[0].ID != src.ID || got[0].Title != "Источник" {
		t.Fatalf("backlinks = %+v, %v", got, err)
	}

	// A self mention is not a backlink.
	self := e.create("", "Сам")
	e.mustSave(self.ID, self.Version, "Сам", para(mention("page", self.ID)))
	if got, _ := e.svc.Backlinks(ctx, self.ID); len(got) != 0 {
		t.Errorf("self backlinks = %+v", got)
	}

	// A property link counts too.
	prop := e.create("", "Свойство")
	e.exec(`INSERT INTO wiki_link (page_id, target_kind, target_id, source) VALUES (?, 'page', ?, 'property')`, prop.ID, target.ID)
	e.exec(`INSERT INTO wiki_link (page_id, target_kind, target_id, source) VALUES (?, 'page', ?, 'mention')`, prop.ID, target.ID)
	if got, _ := e.svc.Backlinks(ctx, target.ID); len(got) != 2 || got[0].Title != "Источник" || got[1].Title != "Свойство" {
		t.Errorf("with property = %+v", got)
	}

	// A trashed source disappears.
	if err := e.svc.Delete(ctx, prop.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Backlinks(ctx, target.ID); len(got) != 1 {
		t.Errorf("after delete = %+v", got)
	}

	// Removing the mention removes the backlink.
	e.mustSave(src.ID, s.Version, "Источник", para(text("пусто")))
	if got, _ := e.svc.Backlinks(ctx, target.ID); len(got) != 0 {
		t.Errorf("after save = %+v", got)
	}

	if err := e.svc.Delete(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Backlinks(ctx, target.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("trashed target: %v", err)
	}
	if _, err := e.svc.Backlinks(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown page: %v", err)
	}
}
