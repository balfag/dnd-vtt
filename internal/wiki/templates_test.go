package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/id"
)

func image(assetID string) string {
	return `{"type":"image","attrs":{"assetId":"` + assetID + `"}}`
}

// templateSource creates a page with an icon, a mention of target and an image.
func (e *env) templateSource(title, target, assetID string) Page {
	e.t.Helper()
	p := e.create("", title)
	content := `{"type":"doc","content":[{"type":"paragraph","content":[` + text("Hello ") + `,` + mention("page", target) + `]},` + image(assetID) + `]}`
	if _, err := e.svc.Save(context.Background(), p.ID, SavePage{Version: p.Version, Title: title, Icon: "\U0001F37A", Content: json.RawMessage(content)}); err != nil {
		e.t.Fatal(err)
	}
	got, err := e.svc.Get(context.Background(), p.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return got
}

func TestSaveTemplateAndCreateFromIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	target := e.create("", "Target")
	aid := e.asset(e.camp)
	src := e.templateSource("NPC", target.ID, aid)

	tpl, err := e.svc.SaveTemplate(ctx, e.camp, src.ID, "NPC card")
	if err != nil {
		t.Fatal(err)
	}
	if tpl.ID == "" || tpl.Name != "NPC card" || tpl.Icon != "\U0001F37A" || !tpl.CreatedAt.Equal(e.clock) {
		t.Errorf("template = %+v", tpl)
	}

	page, err := e.svc.Create(ctx, e.camp, NewPage{Title: "Mirta", TemplateID: tpl.ID})
	if err != nil {
		t.Fatal(err)
	}
	if page.Icon != src.Icon || string(page.Content) != string(src.Content) {
		t.Errorf("page = %q %s, want %q %s", page.Icon, page.Content, src.Icon, src.Content)
	}
	if page.Title != "Mirta" || page.Version != 1 {
		t.Errorf("page = %+v", page)
	}

	// The page is indexed as after an ordinary creation.
	var text, fts string
	e.queryRow(&text, `SELECT content_text FROM wiki_page WHERE id = ?`, page.ID)
	e.queryRow(&fts, `SELECT body FROM wiki_fts WHERE page_id = ?`, page.ID)
	if !strings.Contains(text, "Hello") || fts != text {
		t.Errorf("content_text = %q, fts = %q", text, fts)
	}
	var links, assets int
	e.queryRow(&links, `SELECT COUNT(*) FROM wiki_link WHERE page_id = ? AND target_id = ? AND source = 'mention'`, page.ID, target.ID)
	e.queryRow(&assets, `SELECT COUNT(*) FROM wiki_page_asset WHERE page_id = ? AND asset_id = ?`, page.ID, aid)
	if links != 1 || assets != 1 {
		t.Errorf("links = %d, assets = %d, want 1 and 1", links, assets)
	}

	// The source page changes, the template does not.
	if _, err := e.svc.Save(ctx, src.ID, SavePage{Version: src.Version, Title: "NPC", Content: json.RawMessage(doEmpty())}); err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.Create(ctx, e.camp, NewPage{Title: "Second", TemplateID: tpl.ID})
	if err != nil || string(again.Content) != string(src.Content) {
		t.Errorf("second page = %s, %v", again.Content, err)
	}
}

func doEmpty() string { return `{"type":"doc","content":[{"type":"paragraph"}]}` }

func (e *env) queryRow(dst any, query string, args ...any) {
	e.t.Helper()
	if err := e.db.QueryRow(query, args...).Scan(dst); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
}

func TestCreateFromTemplateRejects(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := e.create("", "Source")
	tpl, err := e.svc.SaveTemplate(ctx, e.camp, src.ID, "T")
	if err != nil {
		t.Fatal(err)
	}
	other := e.campaign("Other")
	otherSrc, err := e.svc.Create(ctx, other, NewPage{Title: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := e.svc.SaveTemplate(ctx, other, otherSrc.ID, "Foreign")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		p    NewPage
		want error
	}{
		{"with content", NewPage{Title: "A", TemplateID: tpl.ID, Content: json.RawMessage(doEmpty())}, ErrInvalid},
		{"with icon", NewPage{Title: "A", TemplateID: tpl.ID, Icon: "x"}, ErrInvalid},
		{"as database", NewPage{Title: "A", TemplateID: tpl.ID, Kind: KindDatabase}, ErrInvalid},
		{"template of another campaign", NewPage{Title: "A", TemplateID: foreign.ID}, ErrNotFound},
		{"unknown template", NewPage{Title: "A", TemplateID: id.New()}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.svc.Create(ctx, e.camp, tt.p); !errors.Is(err, tt.want) {
				t.Errorf("Create() error = %v, want %v", err, tt.want)
			}
		})
	}
	// A JSON null counts as no content.
	if _, err := e.svc.Create(ctx, e.camp, NewPage{Title: "A", TemplateID: tpl.ID, Content: json.RawMessage("null")}); err != nil {
		t.Errorf("Create() with null content: %v", err)
	}
}

func TestSaveTemplateRejects(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	page := e.create("", "Page")
	trashed := e.create("", "Trashed")
	if err := e.svc.Delete(ctx, trashed.ID); err != nil {
		t.Fatal(err)
	}
	db, err := e.svc.Create(ctx, e.camp, NewPage{Title: "DB", Kind: KindDatabase})
	if err != nil {
		t.Fatal(err)
	}
	other := e.campaign("Other")

	tests := []struct {
		name     string
		campaign string
		from     string
		tname    string
		want     error
	}{
		{"unknown campaign", "nope", page.ID, "T", ErrNotFound},
		{"unknown page", e.camp, id.New(), "T", ErrNotFound},
		{"page of another campaign", other, page.ID, "T", ErrNotFound},
		{"trashed page", e.camp, trashed.ID, "T", ErrNotFound},
		{"database", e.camp, db.ID, "T", ErrInvalid},
		{"empty name", e.camp, page.ID, "", ErrInvalid},
		{"long name", e.camp, page.ID, strings.Repeat("x", 101), ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.svc.SaveTemplate(ctx, tt.campaign, tt.from, tt.tname); !errors.Is(err, tt.want) {
				t.Errorf("SaveTemplate() error = %v, want %v", err, tt.want)
			}
		})
	}
	if _, err := e.svc.SaveTemplate(ctx, e.camp, page.ID, strings.Repeat("я", 100)); err != nil {
		t.Errorf("100 code points: %v", err)
	}
}

func TestTemplatesLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	page := e.create("", "Page")
	for i := range MaxTemplates {
		e.exec(`INSERT INTO wiki_template (id, campaign_id, name, icon, content, created_at) VALUES (?, ?, ?, '', ?, 1)`,
			id.New(), e.camp, fmt.Sprintf("T%03d", i), doEmpty())
	}
	if _, err := e.svc.SaveTemplate(ctx, e.camp, page.ID, "One more"); !errors.Is(err, ErrConflict) {
		t.Fatalf("SaveTemplate() error = %v, want ErrConflict", err)
	}
	// The limit is per campaign.
	other := e.campaign("Other")
	op, err := e.svc.Create(ctx, other, NewPage{Title: "P"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveTemplate(ctx, other, op.ID, "Fine"); err != nil {
		t.Errorf("other campaign: %v", err)
	}
}

func TestTemplatesListRenameDelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	page := e.create("", "Page")
	b, err := e.svc.SaveTemplate(ctx, e.camp, page.ID, "Bravo")
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.svc.SaveTemplate(ctx, e.camp, page.ID, "Alpha")
	if err != nil {
		t.Fatal(err)
	}

	list, err := e.svc.Templates(ctx, e.camp)
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("Templates() = %+v, %v", list, err)
	}
	if _, err := e.svc.Templates(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
	if empty, err := e.svc.Templates(ctx, e.campaign("Empty")); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty list = %#v, %v", empty, err)
	}

	renamed, err := e.svc.RenameTemplate(ctx, b.ID, "Aardvark")
	if err != nil || renamed.Name != "Aardvark" || renamed.ID != b.ID {
		t.Fatalf("RenameTemplate() = %+v, %v", renamed, err)
	}
	if list, _ = e.svc.Templates(ctx, e.camp); list[0].ID != b.ID {
		t.Errorf("order after rename = %+v", list)
	}
	for _, name := range []string{"", strings.Repeat("x", 101)} {
		if _, err := e.svc.RenameTemplate(ctx, b.ID, name); !errors.Is(err, ErrInvalid) {
			t.Errorf("rename to %d runes: %v", len(name), err)
		}
	}
	if _, err := e.svc.RenameTemplate(ctx, id.New(), "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename unknown: %v", err)
	}

	if err := e.svc.DeleteTemplate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteTemplate(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if list, _ = e.svc.Templates(ctx, e.camp); len(list) != 1 {
		t.Errorf("after delete = %+v", list)
	}
}

func TestTemplateHoldsItsAssets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	target := e.create("", "Target")
	aid := e.asset(e.camp)
	src := e.templateSource("Src", target.ID, aid)
	tpl, err := e.svc.SaveTemplate(ctx, e.camp, src.ID, "T")
	if err != nil {
		t.Fatal(err)
	}
	// Only the template uses the asset now.
	e.exec(`DELETE FROM wiki_page_asset`)

	if _, err := e.db.Exec(`DELETE FROM asset WHERE id = ?`, aid); err == nil {
		t.Fatal("the asset of a template was deleted")
	}
	if err := e.svc.DeleteTemplate(ctx, tpl.ID); err != nil {
		t.Fatal(err)
	}
	e.exec(`DELETE FROM asset WHERE id = ?`, aid)
}
