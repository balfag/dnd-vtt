package wiki

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/id"
	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// nullable maps "" to NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Service) begin(ctx context.Context, action string) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin: %w", action, err)
	}
	return tx, nil
}

func commit(tx *sql.Tx, action string) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", action, err)
	}
	return nil
}

func campaignExists(ctx context.Context, q querier, campaignID string) error {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM campaign WHERE id = ?`, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find campaign: %w", err)
	}
	return nil
}

func checkTitleIcon(title, icon string) error {
	if !utf8.ValidString(title) {
		return invalid("title: not valid UTF-8")
	}
	if utf8.RuneCountInString(title) > MaxTitle {
		return invalid("title: too long")
	}
	if !doc.ValidIcon(icon) {
		return invalid("icon: invalid")
	}
	return nil
}

// Tree returns the live pages of the campaign except database records,
// ordered by parent and position. Unknown campaign: ErrNotFound.
func (s *Service) Tree(ctx context.Context, campaignID string) ([]PageNode, error) {
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, COALESCE(p.parent_id, ''), p.kind, p.title, p.icon, p.position,
		  CASE p.kind WHEN 'database'
		    THEN (SELECT COUNT(*) FROM wiki_page r WHERE r.parent_id = p.id AND r.deleted_at IS NULL)
		    ELSE 0 END
		FROM wiki_page p LEFT JOIN wiki_page par ON par.id = p.parent_id
		WHERE p.campaign_id = ? AND p.deleted_at IS NULL AND (par.id IS NULL OR par.kind <> 'database')
		ORDER BY COALESCE(p.parent_id, ''), p.position, p.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list wiki tree: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []PageNode{}
	for rows.Next() {
		var n PageNode
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Title, &n.Icon, &n.Position, &n.RecordCount); err != nil {
			return nil, fmt.Errorf("scan wiki page: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wiki tree: %w", err)
	}
	return out, nil
}

// parentInfo checks that parentID may hold pages of the campaign and returns
// its depth (a root page has depth 1).
func parentDepth(ctx context.Context, q querier, campaignID, parentID string) (int, error) {
	var (
		campaign, kind string
		deleted        bool
		grandKind      sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT p.campaign_id, p.kind, p.deleted_at IS NOT NULL, (SELECT g.kind FROM wiki_page g WHERE g.id = p.parent_id)
		FROM wiki_page p WHERE p.id = ?`, parentID).Scan(&campaign, &kind, &deleted, &grandKind)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (campaign != campaignID || deleted)) {
		return 0, invalid("parentId: no such page")
	}
	if err != nil {
		return 0, fmt.Errorf("find parent: %w", err)
	}
	if kind != KindPage || grandKind.String == KindDatabase {
		return 0, invalid("parentId: the parent cannot hold pages")
	}
	return pageDepth(ctx, q, parentID)
}

// pageDepth counts the page and its ancestors.
func pageDepth(ctx context.Context, q querier, pageID string) (int, error) {
	var d int
	err := q.QueryRowContext(ctx, `
		WITH RECURSIVE a (id, parent_id, d) AS (
		  SELECT id, parent_id, 1 FROM wiki_page WHERE id = ?
		  UNION ALL
		  SELECT p.id, p.parent_id, a.d + 1 FROM wiki_page p JOIN a ON p.id = a.parent_id WHERE a.d < ?
		) SELECT COUNT(*) FROM a`, pageID, maxWalk).Scan(&d)
	if err != nil {
		return 0, fmt.Errorf("measure depth: %w", err)
	}
	return d, nil
}

func nextPosition(ctx context.Context, q querier, campaignID, parentID string) (int, error) {
	var pos int
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position) + 1, 0) FROM wiki_page
		WHERE campaign_id = ? AND parent_id IS ? AND deleted_at IS NULL`, campaignID, nullable(parentID)).Scan(&pos)
	if err != nil {
		return 0, fmt.Errorf("find next position: %w", err)
	}
	return pos, nil
}

// isDatabase reports whether pageID is a live database of the campaign.
func isDatabase(ctx context.Context, q querier, campaignID, pageID string) (bool, error) {
	var kind string
	err := q.QueryRowContext(ctx, `
		SELECT kind FROM wiki_page WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL`, pageID, campaignID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find parent: %w", err)
	}
	return kind == KindDatabase, nil
}

// Create adds a page, a database (Kind "database") or, under a database, a record.
func (s *Service) Create(ctx context.Context, campaignID string, p NewPage) (Page, error) {
	tx, err := s.begin(ctx, "create wiki page")
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := campaignExists(ctx, tx, campaignID); err != nil {
		return Page{}, err
	}
	if p.TemplateID != "" {
		if err := applyTemplate(ctx, tx, campaignID, &p); err != nil {
			return Page{}, err
		}
	}
	if err := checkTitleIcon(p.Title, p.Icon); err != nil {
		return Page{}, err
	}
	kind := p.Kind
	switch kind {
	case "":
		kind = KindPage
	case KindPage, KindDatabase:
	default:
		return Page{}, invalid("kind: must be page or database")
	}
	if p.Database != nil && kind != KindDatabase {
		return Page{}, invalid("database: only for kind database")
	}
	if kind == KindDatabase && !isNull(p.Content) {
		return Page{}, invalid("content: a database has no content")
	}

	isRecord := false
	if p.ParentID != "" {
		if isRecord, err = isDatabase(ctx, tx, campaignID, p.ParentID); err != nil {
			return Page{}, err
		}
	}
	var props map[string]json.RawMessage
	if !isNull(p.Props) {
		if err := strictDecode(p.Props, &props, "props"); err != nil {
			return Page{}, err
		}
	}
	switch {
	case isRecord:
		if kind == KindDatabase {
			return Page{}, invalid("kind: a database cannot be inside a database")
		}
		depth, err := pageDepth(ctx, tx, p.ParentID)
		if err != nil {
			return Page{}, err
		}
		if depth+1 > MaxDepth {
			return Page{}, invalid("parentId: deeper than %d levels", MaxDepth)
		}
		n, err := countRows(ctx, tx, `SELECT COUNT(*) FROM wiki_page WHERE parent_id = ? AND deleted_at IS NULL`, p.ParentID)
		if err != nil {
			return Page{}, err
		}
		if n >= MaxRecords {
			return Page{}, fmt.Errorf("%w: the database has %d records", ErrConflict, MaxRecords)
		}
	case len(props) > 0:
		return Page{}, invalid("props: only for a database record")
	case p.ParentID != "":
		depth, err := parentDepth(ctx, tx, campaignID, p.ParentID)
		if err != nil {
			return Page{}, err
		}
		if depth+1 > MaxDepth {
			return Page{}, invalid("parentId: deeper than %d levels", MaxDepth)
		}
	}
	content := p.Content
	if len(content) == 0 || kind == KindDatabase {
		content = doc.Empty()
	}
	res, err := validateContent(ctx, tx, campaignID, content)
	if err != nil {
		return Page{}, err
	}

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM wiki_page WHERE campaign_id = ?`, campaignID).Scan(&total); err != nil {
		return Page{}, fmt.Errorf("count wiki pages: %w", err)
	}
	if total >= MaxPages {
		return Page{}, fmt.Errorf("%w: the campaign has %d pages", ErrConflict, MaxPages)
	}
	pos, err := nextPosition(ctx, tx, campaignID, p.ParentID)
	if err != nil {
		return Page{}, err
	}

	now := s.now().UnixMilli()
	pageID := id.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wiki_page (id, campaign_id, parent_id, kind, title, icon, position, content, content_text, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 1, ?, ?)`,
		pageID, campaignID, nullable(p.ParentID), kind, p.Title, p.Icon, pos, string(res.JSON), now, now); err != nil {
		return Page{}, fmt.Errorf("insert wiki page: %w", err)
	}
	if err := indexPage(ctx, tx, pageID, p.Title, res); err != nil {
		return Page{}, err
	}
	if kind == KindDatabase {
		nd := NewDatabase{}
		if p.Database != nil {
			nd = *p.Database
		}
		if err := createDatabaseParts(ctx, tx, campaignID, pageID, nd); err != nil {
			return Page{}, err
		}
	}
	if isRecord && len(props) > 0 {
		if _, err := s.applyProps(ctx, tx, campaignID, p.ParentID, pageID, props); err != nil {
			return Page{}, err
		}
	}
	page, err := getPage(ctx, tx, pageID)
	if err != nil {
		return Page{}, err
	}
	if err := commit(tx, "create wiki page"); err != nil {
		return Page{}, err
	}
	return page, nil
}
func (s *Service) Get(ctx context.Context, id string) (Page, error) {
	return getPage(ctx, s.db, id)
}

func getPage(ctx context.Context, q querier, pageID string) (Page, error) {
	var (
		p                    Page
		parent, cover        sql.NullString
		parentKind           sql.NullString
		content, props       string
		createdMs, updatedMs int64
	)
	err := q.QueryRowContext(ctx, `
		SELECT p.id, p.campaign_id, p.parent_id, p.kind, (SELECT g.kind FROM wiki_page g WHERE g.id = p.parent_id),
		  p.title, p.icon, p.cover_asset_id, p.content, p.props, p.version, p.created_at, p.updated_at
		FROM wiki_page p WHERE p.id = ? AND p.deleted_at IS NULL`, pageID).
		Scan(&p.ID, &p.CampaignID, &parent, &p.Kind, &parentKind, &p.Title, &p.Icon, &cover, &content, &props,
			&p.Version, &createdMs, &updatedMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("get wiki page: %w", err)
	}
	p.ParentID = parent.String
	p.CoverAssetID = cover.String
	p.IsRecord = parentKind.String == KindDatabase
	p.Content = []byte(content)
	p.Props = []byte(props)
	p.CreatedAt = time.UnixMilli(createdMs)
	p.UpdatedAt = time.UnixMilli(updatedMs)

	if p.Tags, err = pageTags(ctx, q, pageID); err != nil {
		return Page{}, err
	}
	if p.Path, err = pagePath(ctx, q, pageID); err != nil {
		return Page{}, err
	}
	if p.Refs, err = refs(ctx, q, p.CampaignID, pageID); err != nil {
		return Page{}, err
	}
	return p, nil
}

func pageTags(ctx context.Context, q querier, pageID string) ([]Tag, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT t.id, t.name FROM wiki_page_tag pt JOIN wiki_tag t ON t.id = pt.tag_id
		WHERE pt.page_id = ? ORDER BY t.name, t.id`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list page tags: %w", err)
	}
	return out, nil
}

// pagePath returns the ancestors of the page from the root.
func pagePath(ctx context.Context, q querier, pageID string) ([]PageRef, error) {
	rows, err := q.QueryContext(ctx, `
		WITH RECURSIVE a (id, parent_id, title, icon, kind, d) AS (
		  SELECT p.id, p.parent_id, p.title, p.icon, p.kind, 1 FROM wiki_page p
		  WHERE p.id = (SELECT parent_id FROM wiki_page WHERE id = ?)
		  UNION ALL
		  SELECT p.id, p.parent_id, p.title, p.icon, p.kind, a.d + 1 FROM wiki_page p JOIN a ON p.id = a.parent_id WHERE a.d < ?
		) SELECT id, title, icon, kind FROM a ORDER BY d DESC`, pageID, maxWalk)
	if err != nil {
		return nil, fmt.Errorf("list page path: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []PageRef{}
	for rows.Next() {
		var r PageRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Icon, &r.Kind); err != nil {
			return nil, fmt.Errorf("scan page path: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list page path: %w", err)
	}
	return out, nil
}

// livePage is the part of a live page the write operations need.
type livePage struct {
	campaignID, kind string
	parentID         string
	parentKind       string
	version          int
}

func loadLive(ctx context.Context, q querier, pageID string) (livePage, error) {
	var (
		lp                livePage
		parent, parentKnd sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT p.campaign_id, p.kind, p.parent_id, (SELECT g.kind FROM wiki_page g WHERE g.id = p.parent_id), p.version
		FROM wiki_page p WHERE p.id = ? AND p.deleted_at IS NULL`, pageID).
		Scan(&lp.campaignID, &lp.kind, &parent, &parentKnd, &lp.version)
	if errors.Is(err, sql.ErrNoRows) {
		return livePage{}, ErrNotFound
	}
	if err != nil {
		return livePage{}, fmt.Errorf("find wiki page: %w", err)
	}
	lp.parentID, lp.parentKind = parent.String, parentKnd.String
	return lp, nil
}

func (s *Service) Save(ctx context.Context, pageID string, p SavePage) (Saved, error) {
	tx, err := s.begin(ctx, "save wiki page")
	if err != nil {
		return Saved{}, err
	}
	defer func() { _ = tx.Rollback() }()

	lp, err := loadLive(ctx, tx, pageID)
	if err != nil {
		return Saved{}, err
	}
	if p.Version != lp.version && !p.Force {
		return Saved{}, fmt.Errorf("%w: the page is at version %d", ErrConflict, lp.version)
	}
	if err := checkTitleIcon(p.Title, p.Icon); err != nil {
		return Saved{}, err
	}
	if p.CoverAssetID != "" {
		ok, err := assetInCampaign(ctx, tx, lp.campaignID, p.CoverAssetID)
		if err != nil {
			return Saved{}, err
		}
		if !ok {
			return Saved{}, invalid("coverAssetId: the asset does not belong to the campaign")
		}
	}
	res, err := validateContent(ctx, tx, lp.campaignID, p.Content)
	if err != nil {
		return Saved{}, err
	}
	if lp.kind == KindDatabase && !bytes.Equal(res.JSON, doc.Empty()) {
		return Saved{}, invalid("content: a database has no content")
	}

	now := s.now().UnixMilli()
	version := lp.version + 1
	if _, err := tx.ExecContext(ctx, `
		UPDATE wiki_page SET title = ?, icon = ?, cover_asset_id = ?, content = ?, version = ?, updated_at = ?
		WHERE id = ?`, p.Title, p.Icon, nullable(p.CoverAssetID), string(res.JSON), version, now, pageID); err != nil {
		return Saved{}, fmt.Errorf("update wiki page: %w", err)
	}
	if err := indexPage(ctx, tx, pageID, p.Title, res); err != nil {
		return Saved{}, err
	}
	rf, err := refs(ctx, tx, lp.campaignID, pageID)
	if err != nil {
		return Saved{}, err
	}
	if err := commit(tx, "save wiki page"); err != nil {
		return Saved{}, err
	}
	return Saved{Version: version, UpdatedAt: time.UnixMilli(now), Refs: rf}, nil
}

// siblings returns the ids of the live children of parentID ("" = root) of
// the campaign in order, without exclude.
func siblings(ctx context.Context, q querier, campaignID, parentID, exclude string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id FROM wiki_page
		WHERE campaign_id = ? AND parent_id IS ? AND deleted_at IS NULL AND id <> ?
		ORDER BY position, id`, campaignID, nullable(parentID), exclude)
	if err != nil {
		return nil, fmt.Errorf("list siblings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			return nil, fmt.Errorf("scan sibling: %w", err)
		}
		out = append(out, sid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list siblings: %w", err)
	}
	return out, nil
}

// renumber gives the pages the positions 0..n-1 in the given order.
func renumber(ctx context.Context, q querier, ids []string) error {
	for i, sid := range ids {
		if _, err := q.ExecContext(ctx, `UPDATE wiki_page SET position = ? WHERE id = ? AND position <> ?`, i, sid, i); err != nil {
			return fmt.Errorf("renumber pages: %w", err)
		}
	}
	return nil
}

// subtreeHeight is the number of levels of the subtree of the page, itself included.
func subtreeHeight(ctx context.Context, q querier, pageID string) (int, error) {
	var h int
	err := q.QueryRowContext(ctx, `
		WITH RECURSIVE t (id, d) AS (
		  SELECT id, 1 FROM wiki_page WHERE id = ?
		  UNION ALL
		  SELECT p.id, t.d + 1 FROM wiki_page p JOIN t ON p.parent_id = t.id WHERE t.d < ?
		) SELECT MAX(d) FROM t`, pageID, maxWalk).Scan(&h)
	if err != nil {
		return 0, fmt.Errorf("measure subtree: %w", err)
	}
	return h, nil
}

// Move puts the page under parentID ("" = root) at index among its new live
// siblings (index beyond the end = last).
func (s *Service) Move(ctx context.Context, pageID, parentID string, index int) error {
	if index < 0 {
		return invalid("index: negative")
	}
	tx, err := s.begin(ctx, "move wiki page")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lp, err := loadLive(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if lp.parentKind == KindDatabase {
		return invalid("a database record cannot be moved")
	}
	if parentID == pageID {
		return invalid("parentId: a page cannot be moved into itself")
	}
	parentDepthN := 0
	if parentID != "" {
		if parentDepthN, err = parentDepth(ctx, tx, lp.campaignID, parentID); err != nil {
			return err
		}
		var inside int
		err := tx.QueryRowContext(ctx, `
			WITH RECURSIVE a (id, parent_id, d) AS (
			  SELECT id, parent_id, 1 FROM wiki_page WHERE id = ?
			  UNION ALL
			  SELECT p.id, p.parent_id, a.d + 1 FROM wiki_page p JOIN a ON p.id = a.parent_id WHERE a.d < ?
			) SELECT COUNT(*) FROM a WHERE id = ?`, parentID, maxWalk, pageID).Scan(&inside)
		if err != nil {
			return fmt.Errorf("check ancestors: %w", err)
		}
		if inside > 0 {
			return invalid("parentId: a page cannot be moved into its own subtree")
		}
	}
	height, err := subtreeHeight(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if parentDepthN+height > MaxDepth {
		return invalid("parentId: the subtree would be deeper than %d levels", MaxDepth)
	}

	oldSiblings, err := siblings(ctx, tx, lp.campaignID, lp.parentID, pageID)
	if err != nil {
		return err
	}
	newSiblings := oldSiblings
	if parentID != lp.parentID {
		if newSiblings, err = siblings(ctx, tx, lp.campaignID, parentID, pageID); err != nil {
			return err
		}
		if err := renumber(ctx, tx, oldSiblings); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_page SET parent_id = ? WHERE id = ?`, nullable(parentID), pageID); err != nil {
		return fmt.Errorf("move wiki page: %w", err)
	}
	index = min(index, len(newSiblings))
	order := slices.Insert(slices.Clone(newSiblings), index, pageID)
	if err := renumber(ctx, tx, order); err != nil {
		return err
	}
	return commit(tx, "move wiki page")
}

// Delete moves the page and its subtree to the trash.
func (s *Service) Delete(ctx context.Context, pageID string) error {
	tx, err := s.begin(ctx, "delete wiki page")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	lp, err := loadLive(ctx, tx, pageID)
	if err != nil {
		return err
	}
	// Pages that are in the trash already keep their own trash root.
	const subtree = `
		WITH RECURSIVE t (id, d) AS (
		  SELECT id, 1 FROM wiki_page WHERE id = ?1
		  UNION ALL
		  SELECT p.id, t.d + 1 FROM wiki_page p JOIN t ON p.parent_id = t.id WHERE p.deleted_at IS NULL AND t.d < ?2
		)`
	if _, err := tx.ExecContext(ctx, subtree+`
DELETE FROM wiki_fts WHERE page_id IN (SELECT id FROM t)`, pageID, maxWalk); err != nil {
		return fmt.Errorf("remove search rows: %w", err)
	}
	now := s.now().UnixMilli()
	if _, err := tx.ExecContext(ctx, subtree+`
		UPDATE wiki_page SET deleted_at = ?3, trash_root_id = ?1 WHERE id IN (SELECT id FROM t)`, pageID, maxWalk, now); err != nil {
		return fmt.Errorf("trash wiki pages: %w", err)
	}
	rest, err := siblings(ctx, tx, lp.campaignID, lp.parentID, pageID)
	if err != nil {
		return err
	}
	if err := renumber(ctx, tx, rest); err != nil {
		return err
	}
	return commit(tx, "delete wiki page")
}
