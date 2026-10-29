package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/balfag/dnd-vtt/internal/id"
)

// MaxTemplates is the limit of the GM's own page templates per campaign.
const MaxTemplates = 100

type Template struct {
	ID, Name, Icon string
	CreatedAt      time.Time
}

// Templates lists the page templates of the campaign by name.
func (s *Service) Templates(ctx context.Context, campaignID string) ([]Template, error) {
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, icon, created_at FROM wiki_template
		WHERE campaign_id = ? ORDER BY name, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list wiki templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Template{}
	for rows.Next() {
		var (
			t         Template
			createdMs int64
		)
		if err := rows.Scan(&t.ID, &t.Name, &t.Icon, &createdMs); err != nil {
			return nil, fmt.Errorf("scan wiki template: %w", err)
		}
		t.CreatedAt = time.UnixMilli(createdMs)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list wiki templates: %w", err)
	}
	return out, nil
}

func getTemplate(ctx context.Context, q querier, templateID string) (Template, error) {
	var (
		t         Template
		createdMs int64
	)
	err := q.QueryRowContext(ctx, `SELECT id, name, icon, created_at FROM wiki_template WHERE id = ?`, templateID).
		Scan(&t.ID, &t.Name, &t.Icon, &createdMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("get wiki template: %w", err)
	}
	t.CreatedAt = time.UnixMilli(createdMs)
	return t, nil
}

// SaveTemplate copies the icon and content of a live page (not a database);
// name 1..100 code points; MaxTemplates: ErrConflict.
func (s *Service) SaveTemplate(ctx context.Context, campaignID, fromPageID, name string) (Template, error) {
	tx, err := s.begin(ctx, "save wiki template")
	if err != nil {
		return Template{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := campaignExists(ctx, tx, campaignID); err != nil {
		return Template{}, err
	}
	if err := checkName("name", name); err != nil {
		return Template{}, err
	}
	var (
		kind, icon, content string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT kind, icon, content FROM wiki_page WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL`,
		fromPageID, campaignID).Scan(&kind, &icon, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("find template source: %w", err)
	}
	if kind != KindPage {
		return Template{}, invalid("fromPageId: a database cannot become a template")
	}
	n, err := countRows(ctx, tx, `SELECT COUNT(*) FROM wiki_template WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return Template{}, err
	}
	if n >= MaxTemplates {
		return Template{}, fmt.Errorf("%w: the campaign has %d templates", ErrConflict, MaxTemplates)
	}
	res, err := validateContent(ctx, tx, campaignID, []byte(content))
	if err != nil {
		return Template{}, err
	}

	tid := id.New()
	now := s.now().UnixMilli()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wiki_template (id, campaign_id, name, icon, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		tid, campaignID, name, icon, string(res.JSON), now); err != nil {
		return Template{}, fmt.Errorf("insert wiki template: %w", err)
	}
	for _, a := range res.Assets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wiki_template_asset (template_id, asset_id) VALUES (?, ?)`, tid, a); err != nil {
			return Template{}, fmt.Errorf("insert template asset: %w", err)
		}
	}
	t, err := getTemplate(ctx, tx, tid)
	if err != nil {
		return Template{}, err
	}
	if err := commit(tx, "save wiki template"); err != nil {
		return Template{}, err
	}
	return t, nil
}

// RenameTemplate changes the name of a template (1..100 code points).
func (s *Service) RenameTemplate(ctx context.Context, templateID, name string) (Template, error) {
	if err := checkName("name", name); err != nil {
		return Template{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE wiki_template SET name = ? WHERE id = ?`, name, templateID)
	if err != nil {
		return Template{}, fmt.Errorf("rename wiki template: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Template{}, fmt.Errorf("rename wiki template: %w", err)
	} else if n == 0 {
		return Template{}, ErrNotFound
	}
	return getTemplate(ctx, s.db, templateID)
}

// DeleteTemplate removes a template; the assets it used are released.
func (s *Service) DeleteTemplate(ctx context.Context, templateID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM wiki_template WHERE id = ?`, templateID)
	if err != nil {
		return fmt.Errorf("delete wiki template: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete wiki template: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// applyTemplate fills the icon and content of p from the template of the campaign.
func applyTemplate(ctx context.Context, q querier, campaignID string, p *NewPage) error {
	if p.Icon != "" || !isNull(p.Content) {
		return invalid("templateId: icon and content come from the template")
	}
	if p.Kind != "" && p.Kind != KindPage {
		return invalid("templateId: only for a page")
	}
	var icon, content string
	err := q.QueryRowContext(ctx, `SELECT icon, content FROM wiki_template WHERE id = ? AND campaign_id = ?`,
		p.TemplateID, campaignID).Scan(&icon, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find wiki template: %w", err)
	}
	p.Icon, p.Content = icon, []byte(content)
	return nil
}
