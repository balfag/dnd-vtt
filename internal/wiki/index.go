package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

// querier is what *sql.DB and *sql.Tx share.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// validateContent checks raw against the wiki schema. Assets are checked on
// the same transaction that writes the page.
func validateContent(ctx context.Context, q querier, campaignID string, raw []byte) (doc.Result, error) {
	res, err := doc.Validate(raw, doc.Options{AssetOK: func(id string) (bool, error) {
		return assetInCampaign(ctx, q, campaignID, id)
	}})
	if err != nil {
		if errors.Is(err, doc.ErrInvalid) {
			return doc.Result{}, fmt.Errorf("%w: content: %w", ErrInvalid, err)
		}
		return doc.Result{}, fmt.Errorf("validate content: %w", err)
	}
	return res, nil
}

func assetInCampaign(ctx context.Context, q querier, campaignID, assetID string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM asset WHERE id = ? AND campaign_id = ?`, assetID, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find asset: %w", err)
	}
	return true, nil
}

// indexPage rewrites the derived data of a page: the plain text, the search
// row, the mention links and the used assets.
func indexPage(ctx context.Context, q querier, pageID, title string, res doc.Result) error {
	if _, err := q.ExecContext(ctx, `UPDATE wiki_page SET content_text = ? WHERE id = ?`, res.Text, pageID); err != nil {
		return fmt.Errorf("update content text: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM wiki_fts WHERE page_id = ?`, pageID); err != nil {
		return fmt.Errorf("clear search row: %w", err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO wiki_fts (page_id, title, body) VALUES (?, ?, ?)`, pageID, title, res.Text); err != nil {
		return fmt.Errorf("insert search row: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM wiki_link WHERE page_id = ? AND source = 'mention'`, pageID); err != nil {
		return fmt.Errorf("clear links: %w", err)
	}
	for _, m := range res.Mentions {
		if _, err := q.ExecContext(ctx,
			`INSERT OR IGNORE INTO wiki_link (page_id, target_kind, target_id, source) VALUES (?, ?, ?, 'mention')`,
			pageID, m.Kind, m.ID); err != nil {
			return fmt.Errorf("insert link: %w", err)
		}
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM wiki_page_asset WHERE page_id = ?`, pageID); err != nil {
		return fmt.Errorf("clear page assets: %w", err)
	}
	for _, a := range res.Assets {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO wiki_page_asset (page_id, asset_id) VALUES (?, ?)`, pageID, a); err != nil {
			return fmt.Errorf("insert page asset: %w", err)
		}
	}
	return nil
}

// refs returns the current names of the link targets of a page of the campaign.
func refs(ctx context.Context, q querier, campaignID, pageID string) (map[string]RefInfo, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT l.target_kind, l.target_id,
		  CASE l.target_kind
		    WHEN 'page' THEN (SELECT p.title FROM wiki_page p
		                      WHERE p.id = l.target_id AND p.campaign_id = ?1 AND p.deleted_at IS NULL)
		    WHEN 'scene' THEN (SELECT s.name FROM scene s WHERE s.id = l.target_id AND s.campaign_id = ?1)
		    WHEN 'token' THEN (SELECT t.name FROM token t JOIN scene s ON s.id = t.scene_id
		                       WHERE t.id = l.target_id AND s.campaign_id = ?1)
		  END
		FROM wiki_link l WHERE l.page_id = ?2`, campaignID, pageID)
	if err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]RefInfo{}
	for rows.Next() {
		var kind, id string
		var label sql.NullString
		if err := rows.Scan(&kind, &id, &label); err != nil {
			return nil, fmt.Errorf("scan ref: %w", err)
		}
		key := kind + ":" + id
		// Both sources of one target give the same label.
		out[key] = RefInfo{Label: label.String, Exists: label.Valid}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}
	return out, nil
}
