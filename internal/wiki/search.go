package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSearchQuery    = 200 // code points
	defaultSearchHits = 20
	maxSearchHits     = 50
	snippetWords      = 12
)

type SearchQuery struct {
	Q     string // 0..200 code points
	TagID string // "" — no tag filter
	Limit int    // 1..50; 0 means 20
}

type Segment struct {
	Text  string
	Match bool
}

type SearchHit struct {
	ID, Title, Icon, Kind string
	IsRecord              bool
	Path                  []string  // titles of the ancestors from the root; never nil
	Snippet               []Segment // never nil
}

// searchWords keeps the letters, digits and hyphens of every word of q.
// Words without a letter or a digit are dropped.
func searchWords(q string) []string {
	var out []string
	for _, field := range strings.Fields(q) {
		var b strings.Builder
		hasAlnum := false
		for _, r := range field {
			switch {
			case unicode.IsLetter(r) || unicode.IsDigit(r):
				hasAlnum = true
				b.WriteRune(r)
			case r == '-':
				b.WriteRune(r)
			}
		}
		if hasAlnum {
			out = append(out, b.String())
		}
	}
	return out
}

// matchExpr builds the FTS5 query from the cleaned words: a quoted prefix
// term per word, joined by AND.
func matchExpr(words []string) string {
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + w + `"*`
	}
	return strings.Join(terms, " AND ")
}

// parseSnippet cuts the text of snippet() on its \x01 (match start) and
// \x02 (match end) markers.
func parseSnippet(s string) (segs []Segment, matched bool) {
	segs = []Segment{}
	in := false
	for len(s) > 0 {
		i := strings.IndexAny(s, "\x01\x02")
		text := s
		if i >= 0 {
			text = s[:i]
		}
		if text != "" {
			segs = append(segs, Segment{Text: text, Match: in})
			matched = matched || in
		}
		if i < 0 {
			break
		}
		in = s[i] == '\x01'
		s = s[i+1:]
	}
	return segs, matched
}

// Search: empty Q and empty TagID is ErrInvalid. Unknown tag of the campaign: ErrNotFound.
func (s *Service) Search(ctx context.Context, campaignID string, q SearchQuery) ([]SearchHit, error) {
	if !utf8.ValidString(q.Q) {
		return nil, invalid("q: not valid UTF-8")
	}
	if utf8.RuneCountInString(q.Q) > MaxSearchQuery {
		return nil, invalid("q: longer than %d characters", MaxSearchQuery)
	}
	limit := q.Limit
	if limit == 0 {
		limit = defaultSearchHits
	}
	if limit < 1 || limit > maxSearchHits {
		return nil, invalid("limit: must be 1..%d", maxSearchHits)
	}
	words := searchWords(q.Q)
	if len(words) == 0 && q.TagID == "" {
		return nil, invalid("q: nothing to search for")
	}
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return nil, err
	}
	if q.TagID != "" {
		var one int
		err := s.db.QueryRowContext(ctx, `SELECT 1 FROM wiki_tag WHERE id = ? AND campaign_id = ?`, q.TagID, campaignID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("find tag: %w", err)
		}
	}

	hits, err := s.searchRows(ctx, campaignID, words, q.TagID, limit)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		path, err := pagePath(ctx, s.db, hits[i].ID)
		if err != nil {
			return nil, err
		}
		hits[i].Path = make([]string, len(path))
		for j, r := range path {
			hits[i].Path[j] = r.Title
		}
	}
	return hits, nil
}

func (s *Service) searchRows(ctx context.Context, campaignID string, words []string, tagID string, limit int) ([]SearchHit, error) {
	const tagFilter = ` AND EXISTS (SELECT 1 FROM wiki_page_tag pt WHERE pt.page_id = p.id AND pt.tag_id = ?)`
	var (
		query string
		args  []any
	)
	if len(words) > 0 {
		query = `
			SELECT p.id, p.title, p.icon, p.kind,
			  COALESCE((SELECT g.kind = 'database' FROM wiki_page g WHERE g.id = p.parent_id), 0),
			  snippet(wiki_fts, 2, char(1), char(2), '…', ` + fmt.Sprint(snippetWords) + `)
			FROM wiki_fts JOIN wiki_page p ON p.id = wiki_fts.page_id
			WHERE wiki_fts MATCH ? AND p.campaign_id = ? AND p.deleted_at IS NULL`
		args = []any{matchExpr(words), campaignID}
		if tagID != "" {
			query += tagFilter
			args = append(args, tagID)
		}
		query += ` ORDER BY bm25(wiki_fts, 0.0, 5.0, 1.0), p.id LIMIT ?`
	} else {
		query = `
			SELECT p.id, p.title, p.icon, p.kind,
			  COALESCE((SELECT g.kind = 'database' FROM wiki_page g WHERE g.id = p.parent_id), 0),
			  p.content_text
			FROM wiki_page p
			WHERE p.campaign_id = ? AND p.deleted_at IS NULL` + tagFilter + `
			ORDER BY p.title, p.id LIMIT ?`
		args = []any{campaignID, tagID}
	}
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search wiki: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []SearchHit{}
	for rows.Next() {
		var (
			h       SearchHit
			text    string
			isRec   int
			matched bool
		)
		if err := rows.Scan(&h.ID, &h.Title, &h.Icon, &h.Kind, &isRec, &text); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		h.IsRecord = isRec != 0
		if len(words) > 0 {
			h.Snippet, matched = parseSnippet(text)
			if !matched {
				h.Snippet = []Segment{{Text: h.Title}}
			}
		} else {
			h.Snippet = []Segment{{Text: firstWords(text, snippetWords)}}
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search wiki: %w", err)
	}
	return out, nil
}

// firstWords returns the first n whitespace-separated words of s.
func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}
