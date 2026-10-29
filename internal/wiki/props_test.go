package wiki

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func opts(names ...string) json.RawMessage {
	var items []string
	for _, n := range names {
		items = append(items, fmt.Sprintf(`{"name":%q,"color":"blue"}`, n))
	}
	return json.RawMessage(`{"options":[` + strings.Join(items, ",") + `]}`)
}

func variantIDs(t *testing.T, p Property) []string {
	t.Helper()
	var so selectOptions
	if err := json.Unmarshal(p.Options, &so); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, o := range so.Options {
		out = append(out, o.ID)
	}
	return out
}

func TestPropValues(t *testing.T) {
	e := newEnv(t)
	otherCamp := e.campaign("Other")
	otherScene, err := func() (string, error) {
		sid := "other-scene"
		_, err := e.db.Exec(`INSERT INTO scene (id, campaign_id, name, sort, width, height, grid, created_at, updated_at)
			VALUES (?, ?, 'S', 0, 1000, 1000, '{}', 1, 1)`, sid, otherCamp)
		return sid, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	scene := e.scene(e.camp, "Crypt")
	token := e.token(scene, "Goblin")

	targetID, _ := e.newDB("Targets")
	otherDB, _ := e.newDB("Other targets")
	good := e.dbRecord(targetID, "good", "")
	trashed := e.dbRecord(targetID, "trashed", "")
	foreign := e.dbRecord(otherDB, "foreign", "")
	if err := e.svc.Delete(bg, trashed.ID); err != nil {
		t.Fatal(err)
	}

	dbID, p := e.newDB("Main",
		NewProperty{Name: "text", Type: PropText},
		NewProperty{Name: "int", Type: PropNumber, Options: json.RawMessage(`{"format":"integer"}`)},
		NewProperty{Name: "dec", Type: PropNumber},
		NewProperty{Name: "sel", Type: PropSelect, Options: opts("a", "b")},
		NewProperty{Name: "multi", Type: PropMultiSelect, Options: opts("a", "b")},
		NewProperty{Name: "check", Type: PropCheckbox},
		NewProperty{Name: "date", Type: PropDate},
		NewProperty{Name: "url", Type: PropURL},
		NewProperty{Name: "rel", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, targetID))},
		NewProperty{Name: "link", Type: PropGameLink},
	)
	rec := e.dbRecord(dbID, "R", "")
	sel := variantIDs(t, p["sel"])
	multi := variantIDs(t, p["multi"])
	quote := func(s ...string) string {
		b, _ := json.Marshal(s)
		return string(b)
	}
	many := func(n int, f func(i int) string) string {
		items := make([]string, n)
		for i := range items {
			items[i] = f(i)
		}
		return "[" + strings.Join(items, ",") + "]"
	}

	cases := []struct {
		prop string
		good []string
		bad  []string
	}{
		{"text", []string{`""`, `"hi"`, jstr(strings.Repeat("я", 2000))}, []string{`5`, `null2`, jstr(strings.Repeat("я", 2001)), `["a"]`}},
		{"int", []string{`3`, `-7`, `2.0`, `1e2`}, []string{`1.5`, `"3"`, `1e400`, `true`}},
		{"dec", []string{`1.5`, `0`}, []string{`"x"`, `1e400`, `[]`}},
		{"sel", []string{jstr(sel[0])}, []string{`"made-up"`, `5`, quote(sel[0])}},
		{"multi", []string{quote(multi[0]), quote(multi...)}, []string{`[]`, quote(multi[0], multi[0]), quote("nope"), jstr(multi[0]),
			many(21, func(i int) string { return jstr(multi[0]) })}},
		{"check", []string{`true`, `false`}, []string{`"true"`, `1`}},
		{"date", []string{`"2026-02-28"`, `"2024-02-29"`}, []string{`"2026-02-30"`, `"26-1-1"`, `5`, `"2026-1-05"`}},
		{"url", []string{`"https://example.com/a?b=1"`, `"http://x.org"`}, []string{`"ftp://x.org"`, `"example.com"`, `"javascript:alert(1)"`, `5`,
			jstr("https://x.org/" + strings.Repeat("a", 2000))}},
		{"rel", []string{quote(good.ID)}, []string{quote(foreign.ID), quote(trashed.ID), quote("missing"), `[]`, quote(good.ID, good.ID),
			`"` + good.ID + `"`, many(51, func(i int) string { return jstr(good.ID) })}},
		{"link", []string{
			fmt.Sprintf(`[{"kind":"scene","id":%q},{"kind":"token","id":%q}]`, scene, token),
		}, []string{
			fmt.Sprintf(`[{"kind":"scene","id":%q}]`, otherScene), `[]`, `[{"kind":"page","id":"x"}]`,
			fmt.Sprintf(`[{"kind":"scene","id":%q},{"kind":"scene","id":%q}]`, scene, scene),
			fmt.Sprintf(`[{"kind":"scene","id":%q,"x":1}]`, scene),
			fmt.Sprintf(`[{"kind":"token","id":%q}]`, scene),
			many(21, func(i int) string { return fmt.Sprintf(`{"kind":"scene","id":"s%d"}`, i) }),
		}},
	}
	for _, tc := range cases {
		for _, v := range tc.good {
			if _, err := e.setProps(rec.ID, map[string]string{p[tc.prop].ID: v}); err != nil {
				t.Errorf("%s = %.60s: %v", tc.prop, v, err)
			}
		}
		for _, v := range tc.bad {
			_, err := e.setProps(rec.ID, map[string]string{p[tc.prop].ID: v})
			if !isInvalid(err) {
				t.Errorf("%s = %.60s: err = %v, want ErrInvalid", tc.prop, v, err)
			}
		}
	}

	// Unknown keys, a property of another database and a plain page are rejected.
	_, otherProps := e.newDB("Elsewhere", NewProperty{Name: "x", Type: PropText})
	_, err = e.setProps(rec.ID, map[string]string{otherProps["x"].ID: `"v"`})
	wantErr(t, err, ErrInvalid)
	_, err = e.setProps(rec.ID, map[string]string{"nope": `"v"`})
	wantErr(t, err, ErrInvalid)
	_, err = e.setProps(e.create("", "Plain").ID, nil)
	wantErr(t, err, ErrInvalid)
	_, err = e.setProps("missing", nil)
	wantErr(t, err, ErrNotFound)

	// A failed call changes nothing; null deletes a key; other keys stay.
	before, _ := e.svc.Get(bg, rec.ID)
	_, err = e.setProps(rec.ID, map[string]string{p["text"].ID: `"ok"`, p["int"].ID: `1.5`})
	wantErr(t, err, ErrInvalid)
	after, _ := e.svc.Get(bg, rec.ID)
	if string(before.Props) != string(after.Props) {
		t.Errorf("props changed by a failed call: %s -> %s", before.Props, after.Props)
	}
	out, err := e.setProps(rec.ID, map[string]string{p["text"].ID: `null`})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, has := got[p["text"].ID]; has || got[p["check"].ID] != false {
		t.Errorf("props = %s", out)
	}
	if got[p["int"].ID] == nil || got[p["sel"].ID] == nil {
		t.Errorf("untouched keys lost: %s", out)
	}
}

// jstr quotes s as a JSON string.
func jstr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func isInvalid(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrInvalid.Error())
}

func TestOptionsNormalization(t *testing.T) {
	e := newEnv(t)
	dbID, _ := e.newDB("DB")
	add := func(typ string, raw string) (Property, error) {
		var o json.RawMessage
		if raw != "" {
			o = json.RawMessage(raw)
		}
		return e.svc.AddProperty(bg, dbID, NewProperty{Name: fmt.Sprintf("p%d", len(raw)+len(typ)), Type: typ, Options: o})
	}

	for _, tc := range []struct{ typ, in string }{
		{PropNumber, `{"format":"hex"}`}, {PropNumber, `{"x":1}`}, {PropText, `{"x":1}`}, {PropCheckbox, `[]`},
		{PropSelect, `{"options":[{"name":"","color":"red"}]}`},
		{PropSelect, `{"options":[{"name":"a","color":"pink"}]}`},
		{PropSelect, `{"options":[{"id":"made-up","name":"a","color":"red"}]}`},
		{PropSelect, `{"options":[{"name":"a","color":"red","x":1}]}`},
		{PropSelect, `{"extra":[]}`},
		{PropSelect, `{"options":[{"name":"` + strings.Repeat("a", 101) + `","color":"red"}]}`},
		{PropRelation, ``}, {PropRelation, `{}`}, {PropRelation, `{"targetDatabaseId":"missing"}`},
	} {
		if _, err := add(tc.typ, tc.in); !isInvalid(err) {
			t.Errorf("%s %s: err = %v, want ErrInvalid", tc.typ, tc.in, err)
		}
	}
	var many []string
	for i := 0; i <= MaxOptions; i++ {
		many = append(many, fmt.Sprintf(`{"name":"v%d","color":"gray"}`, i))
	}
	if _, err := add(PropSelect, `{"options":[`+strings.Join(many, ",")+`]}`); !isInvalid(err) {
		t.Errorf("101 variants: %v", err)
	}
	if p, err := add(PropSelect, `{"options":[`+strings.Join(many[:MaxOptions], ",")+`]}`); err != nil || len(variantIDs(t, p)) != MaxOptions {
		t.Errorf("100 variants: %v", err)
	}
	if p, err := add(PropSelect, ""); err != nil || string(p.Options) != `{"options":[]}` {
		t.Errorf("default select = %s %v", p.Options, err)
	}
	if p, err := add(PropNumber, `null`); err != nil || string(p.Options) != `{"format":"decimal"}` {
		t.Errorf("default number = %s %v", p.Options, err)
	}
	if p, err := add(PropNumber, `{"format":"integer"}`); err != nil || string(p.Options) != `{"format":"integer"}` {
		t.Errorf("integer number = %s %v", p.Options, err)
	}

	// A relation may point at its own database (via an update) but not at a page or a trashed database.
	page := e.create("", "Plain")
	_, err := e.svc.AddProperty(bg, dbID, NewProperty{Name: "bad", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, page.ID))})
	wantErr(t, err, ErrInvalid)
	trashedDB, _ := e.newDB("Gone")
	if err := e.svc.Delete(bg, trashedDB); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.AddProperty(bg, dbID, NewProperty{Name: "bad", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, trashedDB))})
	wantErr(t, err, ErrInvalid)
	self, err := e.svc.AddProperty(bg, dbID, NewProperty{Name: "self", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, dbID))})
	if err != nil {
		t.Fatal(err)
	}
	r1 := e.dbRecord(dbID, "r1", "")
	r2 := e.dbRecord(dbID, "r2", "")
	if _, err := e.setProps(r1.ID, map[string]string{self.ID: fmt.Sprintf(`[%q]`, r2.ID)}); err != nil {
		t.Errorf("self relation: %v", err)
	}
}

func TestSelectVariantRemoval(t *testing.T) {
	e := newEnv(t)
	dbID, p := e.newDB("DB",
		NewProperty{Name: "sel", Type: PropSelect, Options: opts("a", "b", "c")},
		NewProperty{Name: "multi", Type: PropMultiSelect, Options: opts("a", "b", "c")})
	sel, multi := p["sel"], p["multi"]
	selIDs, multiIDs := variantIDs(t, sel), variantIDs(t, multi)

	cfg := fmt.Sprintf(`{"filters":[{"propertyId":%q,"op":"eq","value":%q},{"propertyId":%q,"op":"empty"},{"propertyId":%q,"op":"contains","value":%q}]}`,
		sel.ID, selIDs[0], sel.ID, multi.ID, multiIDs[2])
	view, err := e.svc.AddView(bg, dbID, "V", ViewTable, json.RawMessage(cfg))
	if err != nil {
		t.Fatal(err)
	}

	onlyA := e.dbRecord(dbID, "onlyA", fmt.Sprintf(`{%q:%q,%q:[%q]}`, sel.ID, selIDs[0], multi.ID, multiIDs[0]))
	mixed := e.dbRecord(dbID, "mixed", fmt.Sprintf(`{%q:%q,%q:[%q,%q]}`, sel.ID, selIDs[1], multi.ID, multiIDs[0], multiIDs[1]))

	// A made-up id is rejected and nothing changes.
	_, err = e.svc.UpdateProperty(bg, sel.ID, PropertyPatch{Options: json.RawMessage(`{"options":[{"id":"made-up","name":"x","color":"red"}]}`)})
	wantErr(t, err, ErrInvalid)

	// Keep b (by id), drop a and c, add a new variant without an id.
	keep := fmt.Sprintf(`{"options":[{"id":%q,"name":"b","color":"blue"},{"name":"new","color":"red"}]}`, selIDs[1])
	updated, err := e.svc.UpdateProperty(bg, sel.ID, PropertyPatch{Options: json.RawMessage(keep)})
	if err != nil {
		t.Fatal(err)
	}
	ids := variantIDs(t, updated)
	if len(ids) != 2 || ids[0] != selIDs[1] || ids[1] == "" || ids[1] == selIDs[0] || ids[1] == selIDs[2] {
		t.Errorf("updated ids = %v", ids)
	}
	got, _ := e.svc.Get(bg, onlyA.ID)
	if strings.Contains(string(got.Props), sel.ID) || !strings.Contains(string(got.Props), multi.ID) {
		t.Errorf("onlyA props = %s", got.Props)
	}
	got, _ = e.svc.Get(bg, mixed.ID)
	if !strings.Contains(string(got.Props), selIDs[1]) {
		t.Errorf("mixed props = %s", got.Props)
	}
	if v := e.view(dbID, view.ID); strings.Contains(string(v.Config), selIDs[0]) ||
		!strings.Contains(string(v.Config), `"op":"empty"`) || !strings.Contains(string(v.Config), multiIDs[2]) {
		t.Errorf("view config = %s", v.Config)
	}

	// Removing every variant of a multi_select drops the key; survivors stay.
	if _, err := e.svc.UpdateProperty(bg, multi.ID, PropertyPatch{Options: json.RawMessage(fmt.Sprintf(`{"options":[{"id":%q,"name":"b","color":"blue"}]}`, multiIDs[1]))}); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.Get(bg, onlyA.ID)
	if strings.Contains(string(got.Props), multi.ID) {
		t.Errorf("empty multi_select kept: %s", got.Props)
	}
	got, _ = e.svc.Get(bg, mixed.ID)
	if !strings.Contains(string(got.Props), fmt.Sprintf(`%q:[%q]`, multi.ID, multiIDs[1])) {
		t.Errorf("mixed multi = %s", got.Props)
	}
	if v := e.view(dbID, view.ID); strings.Contains(string(v.Config), multiIDs[2]) {
		t.Errorf("view keeps removed variant: %s", v.Config)
	}
}

func (e *env) view(dbID, viewID string) View {
	e.t.Helper()
	db, err := e.svc.Database(bg, dbID)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, v := range db.Views {
		if v.ID == viewID {
			return v
		}
	}
	e.t.Fatalf("no view %s", viewID)
	return View{}
}

func (e *env) linkCount(pageID, source string) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM wiki_link WHERE page_id = ? AND source = ?`, pageID, source).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestDeleteProperty(t *testing.T) {
	e := newEnv(t)
	scene := e.scene(e.camp, "Crypt")
	targetID, _ := e.newDB("Targets")
	target := e.dbRecord(targetID, "t", "")
	dbID, p := e.newDB("DB",
		NewProperty{Name: "text", Type: PropText},
		NewProperty{Name: "rel", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, targetID))},
		NewProperty{Name: "link", Type: PropGameLink})
	text, rel, link := p["text"], p["rel"], p["link"]
	r1 := e.dbRecord(dbID, "r1", fmt.Sprintf(`{%q:"x",%q:[%q],%q:[{"kind":"scene","id":%q}]}`, text.ID, rel.ID, target.ID, link.ID, scene))
	r2 := e.dbRecord(dbID, "r2", fmt.Sprintf(`{%q:"y"}`, text.ID))
	trashed := e.dbRecord(dbID, "r3", fmt.Sprintf(`{%q:"z"}`, text.ID))
	if err := e.svc.Delete(bg, trashed.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.linkCount(r1.ID, "property"); n != 2 {
		t.Fatalf("property links = %d, want 2", n)
	}

	cfg := fmt.Sprintf(`{"filters":[{"propertyId":%q,"op":"contains","value":"x"},{"propertyId":%q,"op":"not_empty"}],`+
		`"sorts":[{"propertyId":%q,"dir":"asc"},{"field":"title","dir":"desc"}],`+
		`"visible":[%q,%q,%q],"widths":{%q:100,"title":200}}`,
		text.ID, rel.ID, text.ID, text.ID, rel.ID, link.ID, text.ID)
	view, err := e.svc.AddView(bg, dbID, "V", ViewCards, json.RawMessage(cfg))
	if err != nil {
		t.Fatal(err)
	}

	if err := e.svc.DeleteProperty(bg, text.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{r1.ID, r2.ID, trashed.ID} {
		var props string
		if err := e.db.QueryRow(`SELECT props FROM wiki_page WHERE id = ?`, id).Scan(&props); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(props, text.ID) {
			t.Errorf("record %s keeps the value: %s", id, props)
		}
	}
	v := e.view(dbID, view.ID)
	for _, bad := range []string{`"contains"`, text.ID + `","dir"`, `"visible":["` + text.ID, text.ID + `":100`} {
		if strings.Contains(string(v.Config), bad) {
			t.Errorf("view config keeps %s: %s", bad, v.Config)
		}
	}
	wantCfg := fmt.Sprintf(`{"filters":[{"propertyId":%q,"op":"not_empty"}],"sorts":[{"field":"title","dir":"desc"}],"visible":[%q,%q],"widths":{"title":200}}`,
		rel.ID, rel.ID, link.ID)
	if string(v.Config) != wantCfg {
		t.Errorf("config = %s\nwant     %s", v.Config, wantCfg)
	}

	// Deleting the relation property drops its links; the game link stays.
	if err := e.svc.DeleteProperty(bg, rel.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.linkCount(r1.ID, "property"); n != 1 {
		t.Errorf("property links after delete = %d, want 1", n)
	}
	got, _ := e.svc.Get(bg, r1.ID)
	if _, ok := got.Refs["page:"+target.ID]; ok {
		t.Errorf("refs keep the relation: %+v", got.Refs)
	}
	if _, ok := got.Refs["scene:"+scene]; !ok {
		t.Errorf("refs lost the scene: %+v", got.Refs)
	}
}

func TestRelationLinks(t *testing.T) {
	e := newEnv(t)
	scene := e.scene(e.camp, "Crypt")
	npcs, _ := e.newDB("NPC")
	other, _ := e.newDB("Other")
	boss := e.dbRecord(npcs, "Boss", "")
	otherRec := e.dbRecord(other, "Other rec", "")
	dbID, p := e.newDB("Quests",
		NewProperty{Name: "giver", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, npcs))},
		NewProperty{Name: "place", Type: PropGameLink})
	giver, place := p["giver"], p["place"]
	quest := e.dbRecord(dbID, "Quest", "")

	backlinks := func(id string) []string {
		refs, err := e.svc.Backlinks(bg, id)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range refs {
			out = append(out, r.ID)
		}
		return out
	}
	if got := backlinks(boss.ID); len(got) != 0 {
		t.Fatalf("backlinks = %v", got)
	}
	if _, err := e.setProps(quest.ID, map[string]string{
		giver.ID: fmt.Sprintf(`[%q]`, boss.ID),
		place.ID: fmt.Sprintf(`[{"kind":"scene","id":%q}]`, scene),
	}); err != nil {
		t.Fatal(err)
	}
	if got := backlinks(boss.ID); len(got) != 1 || got[0] != quest.ID {
		t.Errorf("backlinks after set = %v", got)
	}
	page, _ := e.svc.Get(bg, quest.ID)
	if ref := page.Refs["page:"+boss.ID]; !ref.Exists || ref.Label != "Boss" {
		t.Errorf("page ref = %+v", ref)
	}
	if ref := page.Refs["scene:"+scene]; !ref.Exists || ref.Label != "Crypt" {
		t.Errorf("scene ref = %+v", ref)
	}

	// Mentions in the content are not touched by property updates.
	e.exec(`INSERT INTO wiki_link (page_id, target_kind, target_id, source) VALUES (?, 'page', ?, 'mention')`, quest.ID, otherRec.ID)
	if _, err := e.setProps(quest.ID, map[string]string{giver.ID: `null`}); err != nil {
		t.Fatal(err)
	}
	if got := backlinks(boss.ID); len(got) != 0 {
		t.Errorf("backlinks after null = %v", got)
	}
	if n := e.linkCount(quest.ID, "mention"); n != 1 {
		t.Errorf("mention links = %d, want 1", n)
	}
	if n := e.linkCount(quest.ID, "property"); n != 1 {
		t.Errorf("property links = %d, want 1 (the scene)", n)
	}

	// Changing the target database drops the values of the property.
	if _, err := e.setProps(quest.ID, map[string]string{giver.ID: fmt.Sprintf(`[%q]`, boss.ID)}); err != nil {
		t.Fatal(err)
	}
	updated, err := e.svc.UpdateProperty(bg, giver.ID, PropertyPatch{Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, other))})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.Options) != fmt.Sprintf(`{"targetDatabaseId":%q}`, other) {
		t.Errorf("options = %s", updated.Options)
	}
	page, _ = e.svc.Get(bg, quest.ID)
	if strings.Contains(string(page.Props), giver.ID) {
		t.Errorf("value kept after the target changed: %s", page.Props)
	}
	if got := backlinks(boss.ID); len(got) != 0 {
		t.Errorf("backlinks after retarget = %v", got)
	}
	// The same target again keeps values.
	if _, err := e.setProps(quest.ID, map[string]string{giver.ID: fmt.Sprintf(`[%q]`, otherRec.ID)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateProperty(bg, giver.ID, PropertyPatch{Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, other))}); err != nil {
		t.Fatal(err)
	}
	page, _ = e.svc.Get(bg, quest.ID)
	if !strings.Contains(string(page.Props), giver.ID) {
		t.Errorf("value lost with the same target: %s", page.Props)
	}

	// A record put in the trash is not a backlink source any more.
	if err := e.svc.Delete(bg, quest.ID); err != nil {
		t.Fatal(err)
	}
	if got := backlinks(otherRec.ID); len(got) != 0 {
		t.Errorf("trashed source in backlinks: %v", got)
	}
}
