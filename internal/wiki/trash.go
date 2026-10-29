package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TrashTTL is how long a deletion stays in the trash.
const TrashTTL = 30 * 24 * time.Hour

type TrashItem struct {
	ID, Title, Icon, Kind string
	DeletedAt, PurgeAt    time.Time // PurgeAt = DeletedAt + TrashTTL
	PageCount             int       // pages in this deletion, the root included
}

// Trash lists the roots of deletions of the campaign, newest first.
func (s *Service) Trash(ctx context.Context, campaignID string) ([]TrashItem, error) {
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.title, p.icon, p.kind, p.deleted_at,
		  (SELECT COUNT(*) FROM wiki_page c WHERE c.trash_root_id = p.id)
		FROM wiki_page p
		WHERE p.campaign_id = ? AND p.trash_root_id = p.id
		ORDER BY p.deleted_at DESC, p.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list wiki trash: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []TrashItem{}
	for rows.Next() {
		var (
			it      TrashItem
			deleted int64
		)
		if err := rows.Scan(&it.ID, &it.Title, &it.Icon, &it.Kind, &deleted, &it.PageCount); err != nil {
			return nil, fmt.Errorf("scan trash item: %w", err)
		}
		it.DeletedAt = time.UnixMilli(deleted)
		it.PurgeAt = it.DeletedAt.Add(TrashTTL)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wiki trash: %w", err)
	}
	return out, nil
}

// Restore brings the deletion with root id back (ErrNotFound if id is not a trash root).
func (s *Service) Restore(ctx context.Context, rootID string) error {
	tx, err := s.begin(ctx, "restore wiki pages")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		campaignID string
		parent     sql.NullString
	)
	err = tx.QueryRowContext(ctx, `SELECT campaign_id, parent_id FROM wiki_page WHERE id = ?1 AND trash_root_id = ?1`, rootID).
		Scan(&campaignID, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find trash root: %w", err)
	}
	parentID := ""
	if parent.Valid {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM wiki_page WHERE id = ? AND deleted_at IS NULL`, parent.String).Scan(&one)
		if err == nil {
			parentID = parent.String
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("find parent: %w", err)
		}
	}
	pos, err := nextPosition(ctx, tx, campaignID, parentID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_page SET parent_id = ?, position = ? WHERE id = ?`, nullable(parentID), pos, rootID); err != nil {
		return fmt.Errorf("place restored page: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wiki_fts (page_id, title, body)
		SELECT id, title, content_text FROM wiki_page WHERE trash_root_id = ?`, rootID); err != nil {
		return fmt.Errorf("restore search rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_page SET deleted_at = NULL, trash_root_id = NULL WHERE trash_root_id = ?`, rootID); err != nil {
		return fmt.Errorf("restore wiki pages: %w", err)
	}
	return commit(tx, "restore wiki pages")
}

// purgeRoots deletes the subtrees of the roots for good, search rows included.
func purgeRoots(ctx context.Context, q querier, roots []string) (int, error) {
	n := 0
	for _, r := range roots {
		if _, err := q.ExecContext(ctx, `
			WITH RECURSIVE t (id, d) AS (
			  SELECT id, 1 FROM wiki_page WHERE id = ?1
			  UNION ALL
			  SELECT p.id, t.d + 1 FROM wiki_page p JOIN t ON p.parent_id = t.id WHERE t.d < ?2
			) DELETE FROM wiki_fts WHERE page_id IN (SELECT id FROM t)`, r, maxWalk); err != nil {
			return n, fmt.Errorf("remove search rows: %w", err)
		}
		res, err := q.ExecContext(ctx, `DELETE FROM wiki_page WHERE id = ?`, r)
		if err != nil {
			return n, fmt.Errorf("purge wiki pages: %w", err)
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, nil
}

func trashRoots(ctx context.Context, q querier, where string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM wiki_page WHERE trash_root_id = id `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("list trash roots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var rid string
		if err := rows.Scan(&rid); err != nil {
			return nil, fmt.Errorf("scan trash root: %w", err)
		}
		out = append(out, rid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list trash roots: %w", err)
	}
	return out, nil
}

// Purge deletes the deletion with root id for good.
func (s *Service) Purge(ctx context.Context, rootID string) error {
	tx, err := s.begin(ctx, "purge wiki pages")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	roots, err := trashRoots(ctx, tx, `AND id = ?`, rootID)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return ErrNotFound
	}
	if _, err := purgeRoots(ctx, tx, roots); err != nil {
		return err
	}
	return commit(tx, "purge wiki pages")
}

// EmptyTrash deletes every deletion of the campaign for good.
func (s *Service) EmptyTrash(ctx context.Context, campaignID string) error {
	tx, err := s.begin(ctx, "empty wiki trash")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := campaignExists(ctx, tx, campaignID); err != nil {
		return err
	}
	roots, err := trashRoots(ctx, tx, `AND campaign_id = ?`, campaignID)
	if err != nil {
		return err
	}
	if _, err := purgeRoots(ctx, tx, roots); err != nil {
		return err
	}
	return commit(tx, "empty wiki trash")
}

// PurgeExpired deletes for good every deletion older than TrashTTL in all
// campaigns and returns how many roots it removed.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	tx, err := s.begin(ctx, "purge expired wiki pages")
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	cutoff := s.now().Add(-TrashTTL).UnixMilli()
	roots, err := trashRoots(ctx, tx, `AND deleted_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, err := purgeRoots(ctx, tx, roots)
	if err != nil {
		return 0, err
	}
	return n, commit(tx, "purge expired wiki pages")
}
