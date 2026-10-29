package wiki

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func tagNames(tags []Tag) []string {
	out := []string{}
	for _, t := range tags {
		out = append(out, t.Name)
	}
	return out
}

func TestSetTags(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create("", "P")
	q := e.create("", "Q")

	tags, err := e.svc.SetTags(ctx, p.ID, []string{"  Морвен ", "город", "Город"})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagNames(tags); !equalSet(got, []string{"город", "Морвен"}) {
		t.Fatalf("tags = %v", got)
	}
	again, err := e.svc.SetTags(ctx, q.ID, []string{"морвен"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Name != "Морвен" {
		t.Errorf("reused tag = %+v", again)
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_tag`); n != 2 {
		t.Errorf("tags in db = %d", n)
	}
	got, err := e.svc.Get(ctx, p.ID)
	if err != nil || len(got.Tags) != 2 {
		t.Errorf("page tags = %+v, %v", got.Tags, err)
	}
	cleared, err := e.svc.SetTags(ctx, p.ID, nil)
	if err != nil || len(cleared) != 0 {
		t.Errorf("clear = %+v, %v", cleared, err)
	}

	many := make([]string, 21)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	for name, names := range map[string][]string{
		"21 tags":   many,
		"empty":     {" "},
		"too long":  {strings.Repeat("я", 41)},
		"bad utf-8": {"a\xffb"},
	} {
		if _, err := e.svc.SetTags(ctx, p.ID, names); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.SetTags(ctx, p.ID, []string{strings.Repeat("я", 40)}); err != nil {
		t.Errorf("40 code points: %v", err)
	}
	if _, err := e.svc.SetTags(ctx, p.ID, many[:20]); err != nil {
		t.Errorf("20 tags: %v", err)
	}
	if _, err := e.svc.SetTags(ctx, "nope", []string{"a"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown page = %v", err)
	}
	if err := e.svc.Delete(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetTags(ctx, q.ID, []string{"a"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("trashed page = %v", err)
	}
}

func TestTagsCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "A")
	b := e.create("", "B")
	for _, p := range []Page{a, b} {
		if _, err := e.svc.SetTags(ctx, p.ID, []string{"Бета", "альфа"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.svc.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	tags, err := e.svc.Tags(ctx, e.camp)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Name != "альфа" || tags[1].Name != "Бета" || tags[0].PageCount != 1 || tags[1].PageCount != 1 {
		t.Errorf("tags = %+v", tags)
	}
	if _, err := e.svc.Tags(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown campaign = %v", err)
	}
}

func TestRenameTag(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "A")
	b := e.create("", "B")
	ta, _ := e.svc.SetTags(ctx, a.ID, []string{"one", "both"})
	tb, _ := e.svc.SetTags(ctx, b.ID, []string{"Two", "both"})
	one, two := ta[1].ID, tb[1].ID
	if ta[1].Name != "one" || tb[1].Name != "Two" {
		t.Fatalf("setup = %+v %+v", ta, tb)
	}

	got, err := e.svc.RenameTag(ctx, one, " Uno ", false)
	if err != nil || got.ID != one || got.Name != "Uno" {
		t.Fatalf("rename = %+v, %v", got, err)
	}
	if got, err := e.svc.RenameTag(ctx, one, "UNO", false); err != nil || got.Name != "UNO" {
		t.Errorf("case-only rename = %+v, %v", got, err)
	}
	if _, err := e.svc.RenameTag(ctx, one, "two", false); !errors.Is(err, ErrConflict) {
		t.Errorf("taken name = %v", err)
	}
	if _, err := e.svc.RenameTag(ctx, one, "", false); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty name = %v", err)
	}
	if _, err := e.svc.RenameTag(ctx, "nope", "x", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown tag = %v", err)
	}

	// Merge "UNO" into "Two"; the page A has both, B has only Two and both.
	if _, err := e.svc.SetTags(ctx, b.ID, []string{"Two", "UNO", "both"}); err != nil {
		t.Fatal(err)
	}
	merged, err := e.svc.RenameTag(ctx, one, "two", true)
	if err != nil || merged.ID != two || merged.Name != "Two" {
		t.Fatalf("merge = %+v, %v", merged, err)
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_tag WHERE id = ?`, one); n != 0 {
		t.Error("source tag stays")
	}
	for _, p := range []Page{a, b} {
		g, _ := e.svc.Get(ctx, p.ID)
		if got := tagNames(g.Tags); !equalSet(got, []string{"Two", "both"}) {
			t.Errorf("tags of %s = %v", p.Title, got)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM wiki_page_tag WHERE tag_id = ?`, two); n != 2 {
		t.Errorf("pages of the target = %d", n)
	}
}

func TestDeleteTag(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.create("", "A")
	tags, _ := e.svc.SetTags(ctx, a.ID, []string{"x", "y"})
	if err := e.svc.DeleteTag(ctx, tags[0].ID); err != nil {
		t.Fatal(err)
	}
	g, _ := e.svc.Get(ctx, a.ID)
	if got := tagNames(g.Tags); !equalSet(got, []string{"y"}) {
		t.Errorf("tags = %v", got)
	}
	if err := e.svc.DeleteTag(ctx, tags[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v", err)
	}
}
