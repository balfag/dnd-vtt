package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
)

var bg = context.Background()

// newDB creates a database with the properties and returns its page id and the properties by name.
func (e *env) newDB(title string, props ...NewProperty) (string, map[string]Property) {
	e.t.Helper()
	page, err := e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, Title: title, Database: &NewDatabase{Properties: props}})
	if err != nil {
		e.t.Fatalf("create database: %v", err)
	}
	return page.ID, e.props(page.ID)
}

func (e *env) props(dbID string) map[string]Property {
	e.t.Helper()
	db, err := e.svc.Database(bg, dbID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]Property{}
	for _, p := range db.Properties {
		out[p.Name] = p
	}
	return out
}

func (e *env) dbRecord(dbID, title string, props string) Page {
	e.t.Helper()
	np := NewPage{ParentID: dbID, Title: title}
	if props != "" {
		np.Props = json.RawMessage(props)
	}
	p, err := e.svc.Create(bg, e.camp, np)
	if err != nil {
		e.t.Fatalf("create record %q: %v", title, err)
	}
	return p
}

func (e *env) setProps(recordID string, values map[string]string) (json.RawMessage, error) {
	e.t.Helper()
	m := map[string]json.RawMessage{}
	for k, v := range values {
		m[k] = json.RawMessage(v)
	}
	return e.svc.SetProps(bg, recordID, m)
}

func wantErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestCreateDatabase(t *testing.T) {
	e := newEnv(t)
	dbID, props := e.newDB("NPC",
		NewProperty{Name: "Status", Type: PropSelect, Options: json.RawMessage(`{"options":[{"name":"alive","color":"green"}]}`)},
		NewProperty{Name: "Age", Type: PropNumber},
		NewProperty{Name: "Notes", Type: PropText})

	db, err := e.svc.Database(bg, dbID)
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Properties) != 3 || len(db.Views) != 1 || len(db.Records) != 0 {
		t.Fatalf("database = %+v", db)
	}
	for i, name := range []string{"Status", "Age", "Notes"} {
		if db.Properties[i].Name != name || db.Properties[i].Position != i {
			t.Errorf("property %d = %+v", i, db.Properties[i])
		}
	}
	if string(props["Age"].Options) != `{"format":"decimal"}` || string(props["Notes"].Options) != `{}` {
		t.Errorf("default options: %s %s", props["Age"].Options, props["Notes"].Options)
	}
	v := db.Views[0]
	want := fmt.Sprintf(`{"filters":[],"sorts":[],"visible":["%s","%s","%s"],"widths":{}}`,
		db.Properties[0].ID, db.Properties[1].ID, db.Properties[2].ID)
	if v.Name != "Table" || v.Kind != ViewTable || string(v.Config) != want {
		t.Errorf("view = %+v, want config %s", v, want)
	}

	tooMany := make([]NewProperty, MaxProperties+1)
	for i := range tooMany {
		tooMany[i] = NewProperty{Name: fmt.Sprintf("p%d", i), Type: PropText}
	}
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, Database: &NewDatabase{Properties: tooMany}})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, Content: json.RawMessage(emptyDoc)})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, ParentID: dbID})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: "folder"})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Database: &NewDatabase{}})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, Database: &NewDatabase{
		Properties: []NewProperty{{Name: "A", Type: PropText}, {Name: "A", Type: PropText}}}})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, Database: &NewDatabase{
		Properties: []NewProperty{{Name: "A", Type: "bogus"}}}})
	wantErr(t, err, ErrInvalid)
	// A failed creation leaves nothing behind.
	if n := len(e.tree()); n != 1 {
		t.Errorf("tree has %d pages, want 1", n)
	}

	// A database inside a page is fine; two explicit views keep their order.
	parent := e.create("", "Folder")
	page, err := e.svc.Create(bg, e.camp, NewPage{Kind: KindDatabase, ParentID: parent.ID, Database: &NewDatabase{
		Views: []NewView{{Name: "Cards", Kind: ViewCards}, {Name: "Grid", Kind: ViewTable}}}})
	if err != nil {
		t.Fatal(err)
	}
	db, _ = e.svc.Database(bg, page.ID)
	if len(db.Views) != 2 || db.Views[0].Name != "Cards" || db.Views[1].Position != 1 {
		t.Errorf("views = %+v", db.Views)
	}
}

const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

func TestCreateRecord(t *testing.T) {
	e := newEnv(t)
	dbID, props := e.newDB("NPC", NewProperty{Name: "Notes", Type: PropText})
	notes := props["Notes"].ID

	r := e.dbRecord(dbID, "Barliman", fmt.Sprintf(`{"%s":"innkeeper"}`, notes))
	if !r.IsRecord || string(r.Props) != fmt.Sprintf(`{"%s":"innkeeper"}`, notes) {
		t.Errorf("record = %+v", r)
	}
	db, _ := e.svc.Database(bg, dbID)
	if len(db.Records) != 1 || db.Records[0].ID != r.ID || db.Records[0].Tags == nil {
		t.Errorf("records = %+v", db.Records)
	}

	_, err := e.svc.Create(bg, e.camp, NewPage{ParentID: dbID, Database: &NewDatabase{}})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{ParentID: dbID, Props: json.RawMessage(`{"nope":"x"}`)})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{ParentID: dbID, Props: json.RawMessage(fmt.Sprintf(`{"%s":5}`, notes))})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{ParentID: r.ID})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.Create(bg, e.camp, NewPage{Props: json.RawMessage(fmt.Sprintf(`{"%s":"x"}`, notes))})
	wantErr(t, err, ErrInvalid)
	if n := len(e.tree()); n != 1 {
		t.Errorf("failed creations left pages: %d", n)
	}

	// The tree shows the database with its record count, not the records.
	nodes := e.tree()
	if len(nodes) != 1 || nodes[0].ID != dbID || nodes[0].RecordCount != 1 {
		t.Errorf("tree = %+v", nodes)
	}

	e.exec(`
		WITH RECURSIVE n (i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO wiki_page (id, campaign_id, parent_id, kind, title, position, content, content_text, version, created_at, updated_at)
		SELECT 'filler' || i, ?, ?, 'page', 'r', i, '{}', '', 1, 1, 1 FROM n`, MaxRecords-1, e.camp, dbID)
	_, err = e.svc.Create(bg, e.camp, NewPage{ParentID: dbID, Title: "one too many"})
	wantErr(t, err, ErrConflict)

	// Records in the trash do not count.
	if err := e.svc.Delete(bg, r.ID); err != nil {
		t.Fatal(err)
	}
	e.dbRecord(dbID, "fits now", "")
}

func TestPropertyCRUD(t *testing.T) {
	e := newEnv(t)
	dbID, _ := e.newDB("DB")
	a, err := e.svc.AddProperty(bg, dbID, NewProperty{Name: "A", Type: PropText})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := e.svc.AddProperty(bg, dbID, NewProperty{Name: "B", Type: PropCheckbox})
	c, _ := e.svc.AddProperty(bg, dbID, NewProperty{Name: "C", Type: PropDate})
	if a.Position != 0 || b.Position != 1 || c.Position != 2 {
		t.Fatalf("positions = %d %d %d", a.Position, b.Position, c.Position)
	}
	order := func() []string {
		db, _ := e.svc.Database(bg, dbID)
		var names []string
		for i, p := range db.Properties {
			if p.Position != i {
				t.Errorf("position gap: %+v", db.Properties)
			}
			names = append(names, p.Name)
		}
		return names
	}

	two := 2
	if _, err := e.svc.UpdateProperty(bg, a.ID, PropertyPatch{Position: &two}); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []string{"B", "C", "A"}) {
		t.Errorf("order = %v", got)
	}
	name := "Renamed"
	p, err := e.svc.UpdateProperty(bg, b.ID, PropertyPatch{Name: &name})
	if err != nil || p.Name != "Renamed" || p.Type != PropCheckbox {
		t.Errorf("rename = %+v %v", p, err)
	}
	taken := "C"
	_, err = e.svc.UpdateProperty(bg, b.ID, PropertyPatch{Name: &taken})
	wantErr(t, err, ErrInvalid)
	neg := -1
	_, err = e.svc.UpdateProperty(bg, b.ID, PropertyPatch{Position: &neg})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.UpdateProperty(bg, b.ID, PropertyPatch{Options: json.RawMessage(`{"x":1}`)})
	wantErr(t, err, ErrInvalid)
	_, err = e.svc.UpdateProperty(bg, "nope", PropertyPatch{})
	wantErr(t, err, ErrNotFound)

	if err := e.svc.DeleteProperty(bg, b.ID); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []string{"C", "A"}) {
		t.Errorf("order after delete = %v", got)
	}
	wantErr(t, e.svc.DeleteProperty(bg, b.ID), ErrNotFound)

	// Only databases hold properties.
	page := e.create("", "Plain")
	_, err = e.svc.AddProperty(bg, page.ID, NewProperty{Name: "X", Type: PropText})
	wantErr(t, err, ErrNotFound)
	_, err = e.svc.Database(bg, page.ID)
	wantErr(t, err, ErrNotFound)

	for i := 2; i < MaxProperties; i++ {
		if _, err := e.svc.AddProperty(bg, dbID, NewProperty{Name: fmt.Sprintf("p%d", i), Type: PropText}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = e.svc.AddProperty(bg, dbID, NewProperty{Name: "over", Type: PropText})
	wantErr(t, err, ErrConflict)
}
