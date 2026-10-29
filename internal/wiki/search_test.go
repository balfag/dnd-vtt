package wiki

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/id"
	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

// record inserts a record row under the database.
func (e *env) record(parent, title string) string {
	e.t.Helper()
	rid := id.New()
	e.exec(`INSERT INTO wiki_page (id, campaign_id, parent_id, kind, title, position, content, content_text, version, created_at, updated_at)
		VALUES (?, ?, ?, 'page', ?, 0, ?, '', 1, 1, 1)`, rid, e.camp, parent, title, string(doc.Empty()))
	e.exec(`INSERT INTO wiki_fts (page_id, title, body) VALUES (?, ?, '')`, rid, title)
	return rid
}

// body saves a page with the plain paragraph text.
func (e *env) body(p Page, title, txt string) {
	e.t.Helper()
	e.mustSave(p.ID, p.Version, title, para(text(txt)))
}

func (e *env) search(q SearchQuery) []SearchHit {
	e.t.Helper()
	hits, err := e.svc.Search(context.Background(), e.camp, q)
	if err != nil {
		e.t.Fatalf("search %+v: %v", q, err)
	}
	return hits
}

func hitTitles(hits []SearchHit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.Title)
	}
	return out
}

func TestSearchPrefixAndCase(t *testing.T) {
	e := newEnv(t)
	a := e.create("", "Таверна")
	e.body(a, "Таверна", "Здесь подают эль")
	b := e.create("", "Замок")
	e.body(b, "Замок", "Холодный камень")

	for _, q := range []string{"таверн", "ТАВЕРН", "Таверна", "  таверн  "} {
		if got := hitTitles(e.search(SearchQuery{Q: q})); !reflect.DeepEqual(got, []string{"Таверна"}) {
			t.Errorf("q=%q: hits = %v", q, got)
		}
	}
	if got := e.search(SearchQuery{Q: "эль"}); len(got) != 1 || got[0].Title != "Таверна" {
		t.Errorf("body search = %v", hitTitles(got))
	}
}

func TestSearchTwoWordsAnd(t *testing.T) {
	e := newEnv(t)
	a := e.create("", "Таверна Грифон")
	e.body(a, "Таверна Грифон", "место встречи")
	b := e.create("", "Таверна Дракон")
	e.body(b, "Таверна Дракон", "другое место")

	if got := hitTitles(e.search(SearchQuery{Q: "таверн гриф"})); !reflect.DeepEqual(got, []string{"Таверна Грифон"}) {
		t.Errorf("AND hits = %v", got)
	}
	if got := e.search(SearchQuery{Q: "таверн"}); len(got) != 2 {
		t.Errorf("one word hits = %v", hitTitles(got))
	}
}

func TestSearchTitleOutweighsBody(t *testing.T) {
	e := newEnv(t)
	inBody := e.create("", "Aaa")
	e.body(inBody, "Aaa", "рядом стоит дракон")
	inTitle := e.create("", "Дракон")
	e.body(inTitle, "Дракон", "огромный зверь")

	got := hitTitles(e.search(SearchQuery{Q: "дракон"}))
	if !reflect.DeepEqual(got, []string{"Дракон", "Aaa"}) {
		t.Errorf("order = %v", got)
	}
}

func TestSearchScope(t *testing.T) {
	e := newEnv(t)
	live := e.create("", "Тролль живой")
	e.body(live, "Тролль живой", "x")
	gone := e.create("", "Тролль удалённый")
	e.body(gone, "Тролль удалённый", "x")
	if err := e.svc.Delete(context.Background(), gone.ID); err != nil {
		t.Fatal(err)
	}
	other := e.campaign("Other")
	_, err := e.svc.Create(context.Background(), other, NewPage{Title: "Тролль чужой"})
	if err != nil {
		t.Fatal(err)
	}

	if got := hitTitles(e.search(SearchQuery{Q: "тролль"})); !reflect.DeepEqual(got, []string{"Тролль живой"}) {
		t.Errorf("hits = %v", got)
	}
	if _, err := e.svc.Search(context.Background(), "nope", SearchQuery{Q: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
}

func TestSearchTag(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "Гоблин Аа")
	b := e.create("", "Гоблин Бб")
	c := e.create("", "Орк")
	tags, err := e.svc.SetTags(ctx, a.ID, []string{"враг"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetTags(ctx, c.ID, []string{"враг"}); err != nil {
		t.Fatal(err)
	}
	_ = b
	tag := tags[0].ID

	if got := hitTitles(e.search(SearchQuery{Q: "гоблин", TagID: tag})); !reflect.DeepEqual(got, []string{"Гоблин Аа"}) {
		t.Errorf("words+tag = %v", got)
	}
	if got := hitTitles(e.search(SearchQuery{TagID: tag})); !reflect.DeepEqual(got, []string{"Гоблин Аа", "Орк"}) {
		t.Errorf("tag only = %v", got)
	}
	if _, err := e.svc.Search(ctx, e.camp, SearchQuery{TagID: "unknown"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown tag: %v", err)
	}
	other := e.campaign("Other")
	if _, err := e.svc.Search(ctx, other, SearchQuery{TagID: tag}); !errors.Is(err, ErrNotFound) {
		t.Errorf("tag of another campaign: %v", err)
	}
}

func TestSearchSpecialCharacters(t *testing.T) {
	e := newEnv(t)
	p := e.create("", "Тест")
	e.body(p, "Тест", "обычный текст")
	for _, q := range []string{`"`, `*`, `(`, `NEAR`, `a" OR "b`, `NEAR(a b)`, `тест*"`, `-`, `a AND`, `^тест`, `col:тест`, `{тест}`} {
		if _, err := e.svc.Search(context.Background(), e.camp, SearchQuery{Q: q, TagID: ""}); err != nil && !errors.Is(err, ErrInvalid) {
			t.Errorf("q=%q: %v", q, err)
		}
	}
	if got := e.search(SearchQuery{Q: `"тест*(`}); len(got) != 1 {
		t.Errorf("punctuation is dropped: %v", hitTitles(got))
	}
}

func TestSearchInvalid(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for name, q := range map[string]SearchQuery{
		"empty":       {},
		"spaces":      {Q: "   "},
		"symbols":     {Q: `"*()`},
		"too long":    {Q: strings.Repeat("я", 201)},
		"bad utf-8":   {Q: "a\xff"},
		"limit 51":    {Q: "a", Limit: 51},
		"limit minus": {Q: "a", Limit: -1},
	} {
		if _, err := e.svc.Search(ctx, e.camp, q); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := e.svc.Search(ctx, e.camp, SearchQuery{Q: strings.Repeat("я", 200)}); err != nil {
		t.Errorf("200 characters: %v", err)
	}
}

func TestSearchLimit(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 25; i++ {
		e.create("", "Гном")
	}
	if got := e.search(SearchQuery{Q: "гном"}); len(got) != 20 {
		t.Errorf("default limit: %d", len(got))
	}
	if got := e.search(SearchQuery{Q: "гном", Limit: 3}); len(got) != 3 {
		t.Errorf("limit 3: %d", len(got))
	}
}

func TestSearchSnippet(t *testing.T) {
	e := newEnv(t)
	a := e.create("", "Заметка")
	e.body(a, "Заметка", "Старый маяк стоит на скале")
	b := e.create("", "Маяк")
	e.body(b, "Маяк", "обычный текст без совпадений")

	hits := e.search(SearchQuery{Q: "маяк"})
	if len(hits) != 2 {
		t.Fatalf("hits = %v", hitTitles(hits))
	}
	byTitle := map[string]SearchHit{}
	for _, h := range hits {
		byTitle[h.Title] = h
	}
	want := []Segment{{Text: "Старый "}, {Text: "маяк", Match: true}, {Text: " стоит на скале"}}
	if got := byTitle["Заметка"].Snippet; !reflect.DeepEqual(got, want) {
		t.Errorf("body snippet = %+v", got)
	}
	if got := byTitle["Маяк"].Snippet; !reflect.DeepEqual(got, []Segment{{Text: "Маяк"}}) {
		t.Errorf("title snippet = %+v", got)
	}
}

func TestSearchTagOnlySnippetAndPath(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := e.create("", "Корень")
	child := e.create(root.ID, "Дитя")
	e.body(child, "Дитя", "раз два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать тринадцать")
	tags, err := e.svc.SetTags(ctx, child.ID, []string{"т"})
	if err != nil {
		t.Fatal(err)
	}
	hits := e.search(SearchQuery{TagID: tags[0].ID})
	if len(hits) != 1 {
		t.Fatalf("hits = %v", hitTitles(hits))
	}
	want := []Segment{{Text: "раз два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать"}}
	if !reflect.DeepEqual(hits[0].Snippet, want) {
		t.Errorf("snippet = %+v", hits[0].Snippet)
	}
	if !reflect.DeepEqual(hits[0].Path, []string{"Корень"}) {
		t.Errorf("path = %v", hits[0].Path)
	}
	top := e.search(SearchQuery{Q: "корень"})
	if len(top) != 1 || top[0].Path == nil || len(top[0].Path) != 0 || top[0].Snippet == nil {
		t.Errorf("root hit = %+v", top)
	}
}

func TestSearchRecord(t *testing.T) {
	e := newEnv(t)
	db := e.database()
	e.record(db, "Запись")
	hits := e.search(SearchQuery{Q: "запись"})
	if len(hits) != 1 || !hits[0].IsRecord || !reflect.DeepEqual(hits[0].Path, []string{"DB"}) {
		t.Errorf("hits = %+v", hits)
	}
}
