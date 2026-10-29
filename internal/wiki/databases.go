package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/id"
)

// Limits of a database.
const (
	MaxProperties = 30
	MaxViews      = 10
	MaxRecords    = 2000
	MaxOptions    = 100

	maxPropertyName = 100
)

// Property types.
const (
	PropText        = "text"
	PropNumber      = "number"
	PropSelect      = "select"
	PropMultiSelect = "multi_select"
	PropCheckbox    = "checkbox"
	PropDate        = "date"
	PropURL         = "url"
	PropRelation    = "relation"
	PropGameLink    = "game_link"
)

// Kinds of a view.
const (
	ViewTable = "table"
	ViewCards = "cards"
)

type NewProperty struct {
	Name, Type string
	Options    json.RawMessage // nil: the type's default
}

type NewView struct{ Name, Kind string } // Kind: "table" | "cards"

type NewDatabase struct {
	Properties []NewProperty // 0..MaxProperties
	Views      []NewView     // 0..MaxViews; empty: one {"Table", "table"}
}

type Property struct {
	ID, Name, Type string
	Options        json.RawMessage // normalized
	Position       int
}

type View struct {
	ID, Name, Kind string
	Config         json.RawMessage // normalized
	Position       int
}

type Record struct {
	ID, Title, Icon, CoverAssetID string
	Props                         json.RawMessage
	Tags                          []Tag // never nil
	CreatedAt, UpdatedAt          time.Time
}

type Database struct {
	Properties []Property // by position
	Views      []View     // by position
	Records    []Record   // live children by position
}

type PropertyPatch struct {
	Name     *string
	Options  json.RawMessage // nil: unchanged
	Position *int
}

type ViewPatch struct {
	Name     *string
	Config   json.RawMessage // nil: unchanged
	Position *int
}

func checkName(what, name string) error {
	if !utf8.ValidString(name) {
		return invalid("%s: not valid UTF-8", what)
	}
	if n := utf8.RuneCountInString(name); n < 1 || n > maxPropertyName {
		return invalid("%s: must be 1 to %d characters", what, maxPropertyName)
	}
	return nil
}

// loadDatabase returns the campaign of the live database pageID; any other page is ErrNotFound.
func loadDatabase(ctx context.Context, q querier, pageID string) (string, error) {
	lp, err := loadLive(ctx, q, pageID)
	if err != nil {
		return "", err
	}
	if lp.kind != KindDatabase {
		return "", ErrNotFound
	}
	return lp.campaignID, nil
}

func listProperties(ctx context.Context, q querier, dbID string) ([]Property, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, name, type, options, position FROM wiki_property
		WHERE database_page_id = ? ORDER BY position, id`, dbID)
	if err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Property{}
	for rows.Next() {
		var (
			p    Property
			opts string
		)
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &opts, &p.Position); err != nil {
			return nil, fmt.Errorf("scan property: %w", err)
		}
		p.Options = []byte(opts)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list properties: %w", err)
	}
	return out, nil
}

func listViews(ctx context.Context, q querier, dbID string) ([]View, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, name, kind, config, position FROM wiki_view
		WHERE database_page_id = ? ORDER BY position, id`, dbID)
	if err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []View{}
	for rows.Next() {
		var (
			v   View
			cfg string
		)
		if err := rows.Scan(&v.ID, &v.Name, &v.Kind, &cfg, &v.Position); err != nil {
			return nil, fmt.Errorf("scan view: %w", err)
		}
		v.Config = []byte(cfg)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	return out, nil
}

func listRecords(ctx context.Context, q querier, dbID string) ([]Record, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, title, icon, cover_asset_id, props, created_at, updated_at FROM wiki_page
		WHERE parent_id = ? AND deleted_at IS NULL ORDER BY position, id`, dbID)
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Record{}
	byID := map[string]int{}
	for rows.Next() {
		var (
			r                  Record
			cover              sql.NullString
			props              string
			createdMs, updated int64
		)
		if err := rows.Scan(&r.ID, &r.Title, &r.Icon, &cover, &props, &createdMs, &updated); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		r.CoverAssetID = cover.String
		r.Props = []byte(props)
		r.Tags = []Tag{}
		r.CreatedAt = time.UnixMilli(createdMs)
		r.UpdatedAt = time.UnixMilli(updated)
		byID[r.ID] = len(out)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	trows, err := q.QueryContext(ctx, `
		SELECT pt.page_id, t.id, t.name FROM wiki_page_tag pt
		JOIN wiki_tag t ON t.id = pt.tag_id JOIN wiki_page p ON p.id = pt.page_id
		WHERE p.parent_id = ? AND p.deleted_at IS NULL ORDER BY t.name, t.id`, dbID)
	if err != nil {
		return nil, fmt.Errorf("list record tags: %w", err)
	}
	defer func() { _ = trows.Close() }()
	for trows.Next() {
		var (
			pageID string
			t      Tag
		)
		if err := trows.Scan(&pageID, &t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan record tag: %w", err)
		}
		if i, ok := byID[pageID]; ok {
			out[i].Tags = append(out[i].Tags, t)
		}
	}
	if err := trows.Err(); err != nil {
		return nil, fmt.Errorf("list record tags: %w", err)
	}
	return out, nil
}

// Database returns the properties, views and live records of the database pageID.
func (s *Service) Database(ctx context.Context, pageID string) (Database, error) {
	if _, err := loadDatabase(ctx, s.db, pageID); err != nil {
		return Database{}, err
	}
	var (
		db  Database
		err error
	)
	if db.Properties, err = listProperties(ctx, s.db, pageID); err != nil {
		return Database{}, err
	}
	if db.Views, err = listViews(ctx, s.db, pageID); err != nil {
		return Database{}, err
	}
	if db.Records, err = listRecords(ctx, s.db, pageID); err != nil {
		return Database{}, err
	}
	return db, nil
}

func countRows(ctx context.Context, q querier, query string, args ...any) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return n, nil
}

// insertProperty validates np and adds it to the database dbID at position pos.
func insertProperty(ctx context.Context, q querier, campaignID, dbID string, np NewProperty, pos int) (Property, error) {
	if err := checkName("name", np.Name); err != nil {
		return Property{}, err
	}
	if !slices.Contains(propTypes, np.Type) {
		return Property{}, invalid("type: unknown property type %q", np.Type)
	}
	taken, err := countRows(ctx, q, `SELECT COUNT(*) FROM wiki_property WHERE database_page_id = ? AND name = ?`, dbID, np.Name)
	if err != nil {
		return Property{}, err
	}
	if taken > 0 {
		return Property{}, invalid("name: already used in the database")
	}
	opts, err := normalizeOptions(ctx, q, campaignID, np.Type, np.Options, nil)
	if err != nil {
		return Property{}, err
	}
	p := Property{ID: id.New(), Name: np.Name, Type: np.Type, Options: opts, Position: pos}
	if _, err := q.ExecContext(ctx, `
		INSERT INTO wiki_property (id, database_page_id, name, type, options, position) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, dbID, p.Name, p.Type, string(p.Options), p.Position); err != nil {
		return Property{}, fmt.Errorf("insert property: %w", err)
	}
	return p, nil
}

func insertView(ctx context.Context, q querier, dbID, name, kind string, config json.RawMessage, pos int) (View, error) {
	if err := checkName("view name", name); err != nil {
		return View{}, err
	}
	if kind != ViewTable && kind != ViewCards {
		return View{}, invalid("view kind: must be %q or %q", ViewTable, ViewCards)
	}
	props, err := listProperties(ctx, q, dbID)
	if err != nil {
		return View{}, err
	}
	cfg, err := normalizeConfig(config, props)
	if err != nil {
		return View{}, err
	}
	v := View{ID: id.New(), Name: name, Kind: kind, Config: cfg, Position: pos}
	if _, err := q.ExecContext(ctx, `
		INSERT INTO wiki_view (id, database_page_id, name, kind, config, position) VALUES (?, ?, ?, ?, ?, ?)`,
		v.ID, dbID, v.Name, v.Kind, string(v.Config), v.Position); err != nil {
		return View{}, fmt.Errorf("insert view: %w", err)
	}
	return v, nil
}

// createDatabaseParts adds the properties and the views of a new database.
func createDatabaseParts(ctx context.Context, q querier, campaignID, dbID string, nd NewDatabase) error {
	if len(nd.Properties) > MaxProperties {
		return invalid("database.properties: more than %d", MaxProperties)
	}
	if len(nd.Views) > MaxViews {
		return invalid("database.views: more than %d", MaxViews)
	}
	for i, np := range nd.Properties {
		if _, err := insertProperty(ctx, q, campaignID, dbID, np, i); err != nil {
			return err
		}
	}
	views := nd.Views
	if len(views) == 0 {
		views = []NewView{{Name: "Table", Kind: ViewTable}}
	}
	for i, nv := range views {
		if _, err := insertView(ctx, q, dbID, nv.Name, nv.Kind, nil, i); err != nil {
			return err
		}
	}
	return nil
}

// reorder gives the rows of table in the database dbID the positions 0..n-1;
// a non-empty itemID is first moved to pos.
func reorder(ctx context.Context, q querier, table, dbID, itemID string, pos int) error {
	rows, err := q.QueryContext(ctx, `SELECT id FROM `+table+` WHERE database_page_id = ? ORDER BY position, id`, dbID)
	if err != nil {
		return fmt.Errorf("list %s: %w", table, err)
	}
	var ids []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan %s: %w", table, err)
		}
		ids = append(ids, rid)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("list %s: %w", table, err)
	}
	_ = rows.Close()

	if itemID != "" {
		ids = slices.DeleteFunc(ids, func(x string) bool { return x == itemID })
		ids = slices.Insert(ids, min(pos, len(ids)), itemID)
	}
	for i, rid := range ids {
		if _, err := q.ExecContext(ctx, `UPDATE `+table+` SET position = ? WHERE id = ? AND position <> ?`, i, rid, i); err != nil {
			return fmt.Errorf("renumber %s: %w", table, err)
		}
	}
	return nil
}

func getProperty(ctx context.Context, q querier, propID string) (Property, string, error) {
	var (
		p    Property
		dbID string
		opts string
	)
	err := q.QueryRowContext(ctx, `
		SELECT id, database_page_id, name, type, options, position FROM wiki_property WHERE id = ?`, propID).
		Scan(&p.ID, &dbID, &p.Name, &p.Type, &opts, &p.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return Property{}, "", ErrNotFound
	}
	if err != nil {
		return Property{}, "", fmt.Errorf("find property: %w", err)
	}
	p.Options = []byte(opts)
	return p, dbID, nil
}

func getView(ctx context.Context, q querier, viewID string) (View, string, error) {
	var (
		v    View
		dbID string
		cfg  string
	)
	err := q.QueryRowContext(ctx, `
		SELECT id, database_page_id, name, kind, config, position FROM wiki_view WHERE id = ?`, viewID).
		Scan(&v.ID, &dbID, &v.Name, &v.Kind, &cfg, &v.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return View{}, "", ErrNotFound
	}
	if err != nil {
		return View{}, "", fmt.Errorf("find view: %w", err)
	}
	v.Config = []byte(cfg)
	return v, dbID, nil
}

// AddProperty adds a property as the last one. More than MaxProperties: ErrConflict.
func (s *Service) AddProperty(ctx context.Context, pageID string, np NewProperty) (Property, error) {
	tx, err := s.begin(ctx, "add property")
	if err != nil {
		return Property{}, err
	}
	defer func() { _ = tx.Rollback() }()

	campaignID, err := loadDatabase(ctx, tx, pageID)
	if err != nil {
		return Property{}, err
	}
	n, err := countRows(ctx, tx, `SELECT COUNT(*) FROM wiki_property WHERE database_page_id = ?`, pageID)
	if err != nil {
		return Property{}, err
	}
	if n >= MaxProperties {
		return Property{}, fmt.Errorf("%w: the database has %d properties", ErrConflict, MaxProperties)
	}
	p, err := insertProperty(ctx, tx, campaignID, pageID, np, n)
	if err != nil {
		return Property{}, err
	}
	if err := commit(tx, "add property"); err != nil {
		return Property{}, err
	}
	return p, nil
}

// UpdateProperty renames, reconfigures or moves a property; its type never changes.
func (s *Service) UpdateProperty(ctx context.Context, propID string, patch PropertyPatch) (Property, error) {
	tx, err := s.begin(ctx, "update property")
	if err != nil {
		return Property{}, err
	}
	defer func() { _ = tx.Rollback() }()

	p, dbID, err := getProperty(ctx, tx, propID)
	if err != nil {
		return Property{}, err
	}
	campaignID, err := loadDatabase(ctx, tx, dbID)
	if err != nil {
		return Property{}, err
	}

	if patch.Name != nil && *patch.Name != p.Name {
		if err := checkName("name", *patch.Name); err != nil {
			return Property{}, err
		}
		taken, err := countRows(ctx, tx,
			`SELECT COUNT(*) FROM wiki_property WHERE database_page_id = ? AND name = ? AND id <> ?`, dbID, *patch.Name, propID)
		if err != nil {
			return Property{}, err
		}
		if taken > 0 {
			return Property{}, invalid("name: already used in the database")
		}
		p.Name = *patch.Name
	}
	if patch.Position != nil && *patch.Position < 0 {
		return Property{}, invalid("position: negative")
	}

	if patch.Options != nil {
		prev, err := optionIDs(p.Options)
		if err != nil {
			return Property{}, err
		}
		opts, err := normalizeOptions(ctx, tx, campaignID, p.Type, patch.Options, prev)
		if err != nil {
			return Property{}, err
		}
		if err := s.applyOptionChange(ctx, tx, dbID, p, opts, prev); err != nil {
			return Property{}, err
		}
		p.Options = opts
	}

	if _, err := tx.ExecContext(ctx, `UPDATE wiki_property SET name = ?, options = ? WHERE id = ?`, p.Name, string(p.Options), propID); err != nil {
		return Property{}, fmt.Errorf("update property: %w", err)
	}
	if patch.Position != nil {
		if err := reorder(ctx, tx, "wiki_property", dbID, propID, *patch.Position); err != nil {
			return Property{}, err
		}
	}
	out, _, err := getProperty(ctx, tx, propID)
	if err != nil {
		return Property{}, err
	}
	if err := commit(tx, "update property"); err != nil {
		return Property{}, err
	}
	return out, nil
}

// DeleteProperty removes a property with its values and its traces in the views.
func (s *Service) DeleteProperty(ctx context.Context, propID string) error {
	tx, err := s.begin(ctx, "delete property")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, dbID, err := getProperty(ctx, tx, propID)
	if err != nil {
		return err
	}
	if _, err := loadDatabase(ctx, tx, dbID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_property WHERE id = ?`, propID); err != nil {
		return fmt.Errorf("delete property: %w", err)
	}
	if err := reorder(ctx, tx, "wiki_property", dbID, "", 0); err != nil {
		return err
	}
	if err := rewriteRecordProps(ctx, tx, dbID, func(props map[string]any) bool {
		_, had := props[propID]
		delete(props, propID)
		return had
	}); err != nil {
		return err
	}
	if err := rewriteViews(ctx, tx, dbID, func(c *viewConfig) { c.dropProperty(propID) }); err != nil {
		return err
	}
	return commit(tx, "delete property")
}

// AddView adds a view as the last one; nil config: the defaults. More than MaxViews: ErrConflict.
func (s *Service) AddView(ctx context.Context, pageID, name, kind string, config json.RawMessage) (View, error) {
	tx, err := s.begin(ctx, "add view")
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := loadDatabase(ctx, tx, pageID); err != nil {
		return View{}, err
	}
	n, err := countRows(ctx, tx, `SELECT COUNT(*) FROM wiki_view WHERE database_page_id = ?`, pageID)
	if err != nil {
		return View{}, err
	}
	if n >= MaxViews {
		return View{}, fmt.Errorf("%w: the database has %d views", ErrConflict, MaxViews)
	}
	v, err := insertView(ctx, tx, pageID, name, kind, config, n)
	if err != nil {
		return View{}, err
	}
	if err := commit(tx, "add view"); err != nil {
		return View{}, err
	}
	return v, nil
}

// UpdateView renames, reconfigures or moves a view.
func (s *Service) UpdateView(ctx context.Context, viewID string, patch ViewPatch) (View, error) {
	tx, err := s.begin(ctx, "update view")
	if err != nil {
		return View{}, err
	}
	defer func() { _ = tx.Rollback() }()

	v, dbID, err := getView(ctx, tx, viewID)
	if err != nil {
		return View{}, err
	}
	if _, err := loadDatabase(ctx, tx, dbID); err != nil {
		return View{}, err
	}
	if patch.Name != nil {
		if err := checkName("view name", *patch.Name); err != nil {
			return View{}, err
		}
		v.Name = *patch.Name
	}
	if patch.Position != nil && *patch.Position < 0 {
		return View{}, invalid("position: negative")
	}
	if patch.Config != nil {
		props, err := listProperties(ctx, tx, dbID)
		if err != nil {
			return View{}, err
		}
		if v.Config, err = normalizeConfig(patch.Config, props); err != nil {
			return View{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_view SET name = ?, config = ? WHERE id = ?`, v.Name, string(v.Config), viewID); err != nil {
		return View{}, fmt.Errorf("update view: %w", err)
	}
	if patch.Position != nil {
		if err := reorder(ctx, tx, "wiki_view", dbID, viewID, *patch.Position); err != nil {
			return View{}, err
		}
	}
	out, _, err := getView(ctx, tx, viewID)
	if err != nil {
		return View{}, err
	}
	if err := commit(tx, "update view"); err != nil {
		return View{}, err
	}
	return out, nil
}

// DeleteView removes a view; the last view of a database: ErrConflict.
func (s *Service) DeleteView(ctx context.Context, viewID string) error {
	tx, err := s.begin(ctx, "delete view")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, dbID, err := getView(ctx, tx, viewID)
	if err != nil {
		return err
	}
	if _, err := loadDatabase(ctx, tx, dbID); err != nil {
		return err
	}
	n, err := countRows(ctx, tx, `SELECT COUNT(*) FROM wiki_view WHERE database_page_id = ?`, dbID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return fmt.Errorf("%w: a database keeps at least one view", ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_view WHERE id = ?`, viewID); err != nil {
		return fmt.Errorf("delete view: %w", err)
	}
	if err := reorder(ctx, tx, "wiki_view", dbID, "", 0); err != nil {
		return err
	}
	return commit(tx, "delete view")
}
