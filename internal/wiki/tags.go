package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/id"
)

const (
	MaxTagName     = 40 // code points
	MaxTagsPerPage = 20
)

type TagCount struct {
	ID, Name  string
	PageCount int // live pages only
}

func tagKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// cleanTagName trims name and checks its length.
func cleanTagName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) {
		return "", invalid("tag: not valid UTF-8")
	}
	n := utf8.RuneCountInString(name)
	if n < 1 || n > MaxTagName {
		return "", invalid("tag: the name must have 1..%d characters", MaxTagName)
	}
	return name, nil
}

// SetTags replaces the tags of a live page; names are trimmed, 1..40 code
// points, at most 20 distinct (case-insensitively); unknown ones are created.
func (s *Service) SetTags(ctx context.Context, pageID string, names []string) ([]Tag, error) {
	seen := map[string]bool{}
	var clean []string
	for _, raw := range names {
		name, err := cleanTagName(raw)
		if err != nil {
			return nil, err
		}
		if k := tagKey(name); !seen[k] {
			seen[k] = true
			clean = append(clean, name)
		}
	}
	if len(clean) > MaxTagsPerPage {
		return nil, invalid("tags: more than %d", MaxTagsPerPage)
	}

	tx, err := s.begin(ctx, "set wiki tags")
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	lp, err := loadLive(ctx, tx, pageID)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_page_tag WHERE page_id = ?`, pageID); err != nil {
		return nil, fmt.Errorf("clear page tags: %w", err)
	}
	for _, name := range clean {
		key := tagKey(name)
		var tagID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM wiki_tag WHERE campaign_id = ? AND name_key = ?`, lp.campaignID, key).Scan(&tagID)
		if errors.Is(err, sql.ErrNoRows) {
			tagID = id.New()
			_, err = tx.ExecContext(ctx, `INSERT INTO wiki_tag (id, campaign_id, name, name_key) VALUES (?, ?, ?, ?)`, tagID, lp.campaignID, name, key)
		}
		if err != nil {
			return nil, fmt.Errorf("find or create tag: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_page_tag (page_id, tag_id) VALUES (?, ?)`, pageID, tagID); err != nil {
			return nil, fmt.Errorf("insert page tag: %w", err)
		}
	}
	tags, err := orderedPageTags(ctx, tx, pageID)
	if err != nil {
		return nil, err
	}
	if err := commit(tx, "set wiki tags"); err != nil {
		return nil, err
	}
	return tags, nil
}

// orderedPageTags lists the tags of the page by name_key.
func orderedPageTags(ctx context.Context, q querier, pageID string) ([]Tag, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT t.id, t.name FROM wiki_page_tag pt JOIN wiki_tag t ON t.id = pt.tag_id
		WHERE pt.page_id = ? ORDER BY t.name_key, t.id`, pageID)
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

// Tags lists the tags of the campaign by name_key with the number of live pages.
func (s *Service) Tags(ctx context.Context, campaignID string) ([]TagCount, error) {
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name,
		  (SELECT COUNT(*) FROM wiki_page_tag pt JOIN wiki_page p ON p.id = pt.page_id
		   WHERE pt.tag_id = t.id AND p.deleted_at IS NULL)
		FROM wiki_tag t WHERE t.campaign_id = ? ORDER BY t.name_key, t.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list wiki tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.ID, &t.Name, &t.PageCount); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wiki tags: %w", err)
	}
	return out, nil
}

// RenameTag renames; a name taken by another tag of the campaign gives
// ErrConflict unless merge, which moves its pages to that tag, deletes this
// one and returns that tag.
func (s *Service) RenameTag(ctx context.Context, tagID, name string, merge bool) (Tag, error) {
	name, err := cleanTagName(name)
	if err != nil {
		return Tag{}, err
	}
	tx, err := s.begin(ctx, "rename wiki tag")
	if err != nil {
		return Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var campaignID string
	err = tx.QueryRowContext(ctx, `SELECT campaign_id FROM wiki_tag WHERE id = ?`, tagID).Scan(&campaignID)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrNotFound
	}
	if err != nil {
		return Tag{}, fmt.Errorf("find tag: %w", err)
	}
	key := tagKey(name)
	var (
		otherID, otherName string
	)
	err = tx.QueryRowContext(ctx, `SELECT id, name FROM wiki_tag WHERE campaign_id = ? AND name_key = ? AND id <> ?`, campaignID, key, tagID).
		Scan(&otherID, &otherName)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `UPDATE wiki_tag SET name = ?, name_key = ? WHERE id = ?`, name, key, tagID); err != nil {
			return Tag{}, fmt.Errorf("rename tag: %w", err)
		}
		if err := commit(tx, "rename wiki tag"); err != nil {
			return Tag{}, err
		}
		return Tag{ID: tagID, Name: name}, nil
	case err != nil:
		return Tag{}, fmt.Errorf("find tag by name: %w", err)
	case !merge:
		return Tag{}, fmt.Errorf("%w: the tag %q exists", ErrConflict, otherName)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO wiki_page_tag (page_id, tag_id) SELECT page_id, ? FROM wiki_page_tag WHERE tag_id = ?`, otherID, tagID); err != nil {
		return Tag{}, fmt.Errorf("merge tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_tag WHERE id = ?`, tagID); err != nil {
		return Tag{}, fmt.Errorf("delete merged tag: %w", err)
	}
	if err := commit(tx, "rename wiki tag"); err != nil {
		return Tag{}, err
	}
	return Tag{ID: otherID, Name: otherName}, nil
}

// DeleteTag removes the tag from all pages and deletes it.
func (s *Service) DeleteTag(ctx context.Context, tagID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM wiki_tag WHERE id = ?`, tagID)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
