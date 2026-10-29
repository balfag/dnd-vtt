package wiki

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestViewConfigOperators(t *testing.T) {
	e := newEnv(t)
	targetID, _ := e.newDB("T")
	dbID, p := e.newDB("DB",
		NewProperty{Name: "text", Type: PropText},
		NewProperty{Name: "url", Type: PropURL},
		NewProperty{Name: "num", Type: PropNumber},
		NewProperty{Name: "sel", Type: PropSelect, Options: opts("a")},
		NewProperty{Name: "multi", Type: PropMultiSelect, Options: opts("a")},
		NewProperty{Name: "check", Type: PropCheckbox},
		NewProperty{Name: "date", Type: PropDate},
		NewProperty{Name: "rel", Type: PropRelation, Options: json.RawMessage(fmt.Sprintf(`{"targetDatabaseId":%q}`, targetID))},
		NewProperty{Name: "link", Type: PropGameLink},
	)
	selV, multiV := variantIDs(t, p["sel"])[0], variantIDs(t, p["multi"])[0]

	type c struct {
		prop, op, value string // value "" means no value; prop "field:x" is a built-in field
	}
	valid := map[string][]c{
		"text": {{"text", "contains", `"ab"`}, {"text", "empty", ""}, {"text", "not_empty", ""},
			{"url", "contains", `"http"`}, {"url", "not_empty", ""}, {"field:title", "contains", `"x"`}, {"field:title", "empty", ""}},
		"number": {{"num", "eq", `1`}, {"num", "neq", `-1.5`}, {"num", "lt", `0`}, {"num", "lte", `0`}, {"num", "gt", `1e3`}, {"num", "gte", `2`}, {"num", "empty", ""}},
		"select": {{"sel", "eq", jstr(selV)}, {"sel", "neq", jstr(selV)}, {"sel", "empty", ""},
			{"multi", "contains", jstr(multiV)}, {"multi", "not_contains", jstr(multiV)}, {"multi", "empty", ""}},
		"check": {{"check", "checked", ""}, {"check", "unchecked", ""}},
		"date": {{"date", "before", `"2026-01-31"`}, {"date", "after", `"2026-01-31"`}, {"date", "eq", `"2026-01-31"`}, {"date", "empty", ""},
			{"field:createdAt", "before", `"2026-01-31"`}, {"field:updatedAt", "eq", `"2026-01-31"`}},
		"links": {{"rel", "empty", ""}, {"rel", "not_empty", ""}, {"link", "empty", ""}, {"link", "not_empty", ""}},
		"tags":  {{"field:tags", "contains", `"tag-id"`}},
	}
	invalid := []c{
		{"text", "lt", `"a"`}, {"text", "contains", ""}, {"text", "contains", `""`}, {"text", "contains", `5`},
		{"text", "contains", jstr(strings.Repeat("a", 201))}, {"text", "empty", `"x"`}, {"text", "eq", `"x"`},
		{"num", "contains", `1`}, {"num", "eq", `"1"`}, {"num", "eq", `1e400`}, {"num", "eq", ""},
		{"sel", "contains", jstr(selV)}, {"sel", "eq", `"made-up"`}, {"multi", "eq", jstr(multiV)}, {"multi", "contains", `"made-up"`},
		{"check", "contains", `"x"`}, {"check", "eq", `true`}, {"check", "checked", `true`},
		{"date", "contains", `"2026"`}, {"date", "before", `"2026-02-30"`}, {"date", "before", `5`},
		{"rel", "contains", `"x"`}, {"link", "eq", ""}, {"rel", "empty", `1`},
		{"field:title", "before", `"2026-01-01"`}, {"field:createdAt", "empty", ""}, {"field:createdAt", "lt", `"2026-01-01"`},
		{"field:tags", "empty", ""}, {"field:tags", "contains", `5`}, {"field:nope", "empty", ""},
		{"nope-id", "empty", ""},
	}

	build := func(x c) string {
		key := "propertyId"
		target := x.prop
		if name, ok := strings.CutPrefix(x.prop, "field:"); ok {
			key, target = "field", name
		} else if pr, ok := p[x.prop]; ok {
			target = pr.ID
		}
		f := fmt.Sprintf(`{%q:%q,"op":%q`, key, target, x.op)
		if x.value != "" {
			f += `,"value":` + x.value
		}
		return `{"filters":[` + f + `}]}`
	}
	for group, cases := range valid {
		for _, x := range cases {
			v, err := e.svc.AddView(bg, dbID, "v", ViewTable, json.RawMessage(build(x)))
			if err != nil {
				t.Errorf("%s: %+v: %v", group, x, err)
				continue
			}
			if err := e.svc.DeleteView(bg, v.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, x := range invalid {
		_, err := e.svc.AddView(bg, dbID, "v", ViewTable, json.RawMessage(build(x)))
		if !isInvalid(err) {
			t.Errorf("%+v: err = %v, want ErrInvalid", x, err)
		}
	}
}

func TestViewConfigShape(t *testing.T) {
	e := newEnv(t)
	dbID, p := e.newDB("DB", NewProperty{Name: "a", Type: PropText}, NewProperty{Name: "b", Type: PropNumber})
	a, b := p["a"].ID, p["b"].ID

	// Defaults: every property visible, in order.
	v, err := e.svc.AddView(bg, dbID, "Defaults", ViewCards, nil)
	if err != nil || string(v.Config) != fmt.Sprintf(`{"filters":[],"sorts":[],"visible":[%q,%q],"widths":{}}`, a, b) {
		t.Errorf("default config = %s %v", v.Config, err)
	}
	v, err = e.svc.AddView(bg, dbID, "Partial", ViewTable, json.RawMessage(`{"visible":[]}`))
	if err != nil || !strings.Contains(string(v.Config), `"visible":[]`) {
		t.Errorf("empty visible = %s %v", v.Config, err)
	}
	full := fmt.Sprintf(`{"filters":[],"sorts":[{"propertyId":%q,"dir":"desc"},{"field":"updatedAt","dir":"asc"},{"field":"title","dir":"asc"}],"visible":[%q],"widths":{%q:2000,"title":40}}`, a, b, b)
	if v, err = e.svc.AddView(bg, dbID, "Full", ViewTable, json.RawMessage(full)); err != nil {
		t.Errorf("full config: %v", err)
	} else if string(v.Config) != full {
		t.Errorf("config = %s\nwant     %s", v.Config, full)
	}

	for name, cfg := range map[string]string{
		"extra key":        `{"filters":[],"group":1}`,
		"extra filter key": fmt.Sprintf(`{"filters":[{"propertyId":%q,"op":"empty","x":1}]}`, a),
		"four sorts":       `{"sorts":[{"field":"title","dir":"asc"},{"field":"title","dir":"asc"},{"field":"title","dir":"asc"},{"field":"title","dir":"asc"}]}`,
		"bad dir":          `{"sorts":[{"field":"title","dir":"up"}]}`,
		"sort by tags":     `{"sorts":[{"field":"tags","dir":"asc"}]}`,
		"both keys":        fmt.Sprintf(`{"sorts":[{"propertyId":%q,"field":"title","dir":"asc"}]}`, a),
		"no key":           `{"sorts":[{"dir":"asc"}]}`,
		"unknown sort":     `{"sorts":[{"propertyId":"nope","dir":"asc"}]}`,
		"21 filters":       `{"filters":[` + strings.TrimSuffix(strings.Repeat(`{"field":"title","op":"empty"},`, 21), ",") + `]}`,
		"unknown visible":  `{"visible":["nope"]}`,
		"repeated visible": fmt.Sprintf(`{"visible":[%q,%q]}`, a, a),
		"narrow width":     fmt.Sprintf(`{"widths":{%q:39}}`, a),
		"wide width":       fmt.Sprintf(`{"widths":{%q:2001}}`, a),
		"fractional width": fmt.Sprintf(`{"widths":{%q:50.5}}`, a),
		"unknown width":    `{"widths":{"nope":100}}`,
		"not an object":    `[]`,
	} {
		if _, err := e.svc.AddView(bg, dbID, "bad", ViewTable, json.RawMessage(cfg)); !isInvalid(err) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	twenty := `{"filters":[` + strings.TrimSuffix(strings.Repeat(`{"field":"title","op":"empty"},`, 20), ",") + `]}`
	if _, err := e.svc.AddView(bg, dbID, "twenty", ViewTable, json.RawMessage(twenty)); err != nil {
		t.Errorf("20 filters: %v", err)
	}

	_, err = e.svc.AddView(bg, dbID, "", ViewTable, nil)
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.AddView(bg, dbID, "x", "gallery", nil)
	wantErr(t, err, ErrInvalid)
}

func TestViewCRUD(t *testing.T) {
	e := newEnv(t)
	dbID, p := e.newDB("DB", NewProperty{Name: "a", Type: PropText})
	first := e.svc
	db, _ := first.Database(bg, dbID)
	table := db.Views[0]
	cards, err := e.svc.AddView(bg, dbID, "Cards", ViewCards, nil)
	if err != nil {
		t.Fatal(err)
	}
	third, _ := e.svc.AddView(bg, dbID, "Third", ViewTable, nil)
	if table.Position != 0 || cards.Position != 1 || third.Position != 2 {
		t.Fatalf("positions = %d %d %d", table.Position, cards.Position, third.Position)
	}
	names := func() []string {
		db, _ := e.svc.Database(bg, dbID)
		var out []string
		for i, v := range db.Views {
			if v.Position != i {
				t.Errorf("position gap: %+v", db.Views)
			}
			out = append(out, v.Name)
		}
		return out
	}

	zero := 0
	if _, err := e.svc.UpdateView(bg, third.ID, ViewPatch{Position: &zero}); err != nil {
		t.Fatal(err)
	}
	if got := names(); !slices.Equal(got, []string{"Third", "Table", "Cards"}) {
		t.Errorf("order = %v", got)
	}
	name := "Renamed"
	cfg := fmt.Sprintf(`{"widths":{%q:300}}`, p["a"].ID)
	v, err := e.svc.UpdateView(bg, cards.ID, ViewPatch{Name: &name, Config: json.RawMessage(cfg)})
	if err != nil || v.Name != "Renamed" || v.Kind != ViewCards || !strings.Contains(string(v.Config), `300`) {
		t.Errorf("update = %+v %v", v, err)
	}
	if v, _ = e.svc.UpdateView(bg, cards.ID, ViewPatch{}); v.Name != "Renamed" || !strings.Contains(string(v.Config), `300`) {
		t.Errorf("empty patch changed the view: %+v", v)
	}
	_, err = e.svc.UpdateView(bg, cards.ID, ViewPatch{Config: json.RawMessage(`{"nope":1}`)})
	wantErr(t, err, ErrInvalid)
	empty := ""
	_, err = e.svc.UpdateView(bg, cards.ID, ViewPatch{Name: &empty})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.UpdateView(bg, "nope", ViewPatch{})
	wantErr(t, err, ErrNotFound)

	for _, id := range []string{third.ID, table.ID} {
		if err := e.svc.DeleteView(bg, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := names(); !slices.Equal(got, []string{"Renamed"}) {
		t.Errorf("order after delete = %v", got)
	}
	wantErr(t, e.svc.DeleteView(bg, cards.ID), ErrConflict)
	wantErr(t, e.svc.DeleteView(bg, "nope"), ErrNotFound)

	for i := 1; i < MaxViews; i++ {
		if _, err := e.svc.AddView(bg, dbID, fmt.Sprintf("v%d", i), ViewTable, nil); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.AddView(bg, dbID, "over", ViewTable, nil)
	wantErr(t, err, ErrConflict)
	_, err = e.svc.AddView(bg, e.create("", "Plain").ID, "x", ViewTable, nil)
	wantErr(t, err, ErrNotFound)
}
