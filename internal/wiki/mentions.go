package wiki

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	MaxMentionQuery = 100 // code points
	maxMentionHits  = 20
	recentMentions  = 10
)

type MentionHit struct{ Kind, ID, Label, Detail string }

// MentionSearch finds up to 20 pages (records included), scenes and tokens of
// the campaign; q is 0..100 code points.
func (s *Service) MentionSearch(ctx context.Context, campaignID, q string) ([]MentionHit, error) {
	if !utf8.ValidString(q) {
		return nil, invalid("q: not valid UTF-8")
	}
	if utf8.RuneCountInString(q) > MaxMentionQuery {
		return nil, invalid("q: longer than %d characters", MaxMentionQuery)
	}
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))

	if q == "" {
		return s.recentPages(ctx, campaignID)
	}

	var cands []MentionHit
	for _, src := range []struct{ kind, query string }{
		{"page", `
			SELECT p.id, p.title, COALESCE((SELECT g.title FROM wiki_page g WHERE g.id = p.parent_id), '')
			FROM wiki_page p WHERE p.campaign_id = ? AND p.deleted_at IS NULL`},
		{"scene", `SELECT id, name, '' FROM scene WHERE campaign_id = ?`},
		{"token", `
			SELECT t.id, t.name, s.name FROM token t JOIN scene s ON s.id = t.scene_id
			WHERE s.campaign_id = ? AND t.name <> ''`},
	} {
		hits, err := s.mentionRows(ctx, src.kind, src.query, campaignID)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if strings.Contains(strings.ToLower(h.Label), q) {
				cands = append(cands, h)
			}
		}
	}

	type keyed struct {
		hit    MentionHit
		prefix bool
		name   string
	}
	ks := make([]keyed, len(cands))
	for i, h := range cands {
		name := strings.ToLower(h.Label)
		ks[i] = keyed{hit: h, prefix: strings.HasPrefix(name, q), name: name}
	}
	sort.SliceStable(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.prefix != b.prefix {
			return a.prefix
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.hit.Kind < b.hit.Kind
	})
	if len(ks) > maxMentionHits {
		ks = ks[:maxMentionHits]
	}
	out := make([]MentionHit, len(ks))
	for i, k := range ks {
		out[i] = k.hit
	}
	return out, nil
}

func (s *Service) mentionRows(ctx context.Context, kind, query, campaignID string) ([]MentionHit, error) {
	rows, err := s.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list %s mentions: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	var out []MentionHit
	for rows.Next() {
		h := MentionHit{Kind: kind}
		if err := rows.Scan(&h.ID, &h.Label, &h.Detail); err != nil {
			return nil, fmt.Errorf("scan %s mention: %w", kind, err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %s mentions: %w", kind, err)
	}
	return out, nil
}

func (s *Service) recentPages(ctx context.Context, campaignID string) ([]MentionHit, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.title, COALESCE((SELECT g.title FROM wiki_page g WHERE g.id = p.parent_id), '')
		FROM wiki_page p WHERE p.campaign_id = ? AND p.deleted_at IS NULL
		ORDER BY p.updated_at DESC, p.id LIMIT ?`, campaignID, recentMentions)
	if err != nil {
		return nil, fmt.Errorf("list recent pages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []MentionHit{}
	for rows.Next() {
		h := MentionHit{Kind: KindPage}
		if err := rows.Scan(&h.ID, &h.Label, &h.Detail); err != nil {
			return nil, fmt.Errorf("scan recent page: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent pages: %w", err)
	}
	return out, nil
}

// Backlinks lists live pages linking to the live page id (any wiki_link source), by title.
func (s *Service) Backlinks(ctx context.Context, id string) ([]PageRef, error) {
	if _, err := loadLive(ctx, s.db, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT p.id, p.title, p.icon, p.kind FROM wiki_link l JOIN wiki_page p ON p.id = l.page_id
		WHERE l.target_kind = 'page' AND l.target_id = ? AND p.deleted_at IS NULL AND p.id <> ?
		ORDER BY p.title, p.id`, id, id)
	if err != nil {
		return nil, fmt.Errorf("list backlinks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []PageRef{}
	for rows.Next() {
		var r PageRef
		if err := rows.Scan(&r.ID, &r.Title, &r.Icon, &r.Kind); err != nil {
			return nil, fmt.Errorf("scan backlink: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list backlinks: %w", err)
	}
	return out, nil
}
