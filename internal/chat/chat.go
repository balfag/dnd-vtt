// Package chat stores chat messages and rolls and renders them per recipient.
package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/dice"
	"github.com/balfag/dnd-vtt/internal/id"
	"github.com/balfag/dnd-vtt/internal/ws"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

var (
	ErrNotFound = errors.New("chat: not found")
	ErrInvalid  = errors.New("chat: invalid message")
)

// Limits of a message (protocol.md, "Chat and rolls").
const (
	maxTextRunes  = 2000
	maxLabelRunes = 100
	maxRecipients = 20
	snapshotLimit = 50
	maxHistory    = 100
)

// Kinds and visibilities of a message.
const (
	kindText = "text"
	kindRoll = "roll"

	visPublic  = "public"
	visPrivate = "private"
	visGM      = "gm"
	visSelf    = "self"
	visBlind   = "blind"
)

type Options struct{ Now func() time.Time }

// Service is safe for concurrent use.
type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Render returns m as v must see it; ok=false when v must not see it at all.
// m is not changed: the result of a blind roll is cleared in a copy.
func Render(m protocol.ChatMessage, v ws.Viewer) (out protocol.ChatMessage, ok bool) {
	author := m.AuthorPlayerID != nil && v.PlayerID != "" && *m.AuthorPlayerID == v.PlayerID
	switch m.Visibility {
	case visPublic:
		return m, true
	case visPrivate:
		if author || (v.PlayerID != "" && slices.Contains(m.To, v.PlayerID)) {
			return m, true
		}
	case visGM:
		if author || v.IsGM {
			return m, true
		}
	case visSelf:
		if author {
			return m, true
		}
	case visBlind:
		if v.IsGM {
			return m, true
		}
		if author {
			if m.Roll != nil {
				roll := *m.Roll
				roll.Result = nil
				m.Roll = &roll
			}
			return m, true
		}
	}
	return protocol.ChatMessage{}, false
}

// SendText stores a text message of the author; to makes it private.
func (s *Service) SendText(ctx context.Context, campaignID, gameSessionID, authorID, text string, to []string) (protocol.ChatMessage, error) {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n < 1 || n > maxTextRunes {
		return protocol.ChatMessage{}, invalid("text must be 1..%d characters", maxTextRunes)
	}
	if len(to) > maxRecipients {
		return protocol.ChatMessage{}, invalid("at most %d recipients", maxRecipients)
	}
	for i, p := range to {
		if p == authorID {
			return protocol.ChatMessage{}, invalid("the author cannot be a recipient")
		}
		if slices.Contains(to[:i], p) {
			return protocol.ChatMessage{}, invalid("duplicate recipient")
		}
	}
	if len(to) > 0 {
		if err := s.checkRecipients(ctx, campaignID, to); err != nil {
			return protocol.ChatMessage{}, err
		}
	}

	m := protocol.ChatMessage{AuthorPlayerID: &authorID, Kind: kindText, Visibility: visPublic, Text: text}
	if len(to) > 0 {
		m.Visibility = visPrivate
		m.To = slices.Clone(to)
	}
	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{text})
	if err != nil {
		return protocol.ChatMessage{}, err
	}
	return s.insert(ctx, m, campaignID, gameSessionID, payload)
}

// checkRecipients requires every id to be a present participant of the campaign.
func (s *Service) checkRecipients(ctx context.Context, campaignID string, to []string) error {
	args := make([]any, 0, len(to)+1)
	args = append(args, campaignID)
	for _, p := range to {
		args = append(args, p)
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM player WHERE campaign_id = ? AND removed = 0 AND id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(to)), ",")+`)`, args...).Scan(&n)
	if err != nil {
		return fmt.Errorf("check recipients: %w", err)
	}
	if n != len(to) {
		return ErrNotFound
	}
	return nil
}

// SendRoll rolls formula with roller and stores the roll of the author.
// A formula that does not parse is returned as the *dice.ParseError.
func (s *Service) SendRoll(ctx context.Context, campaignID, gameSessionID, authorID, formula, visibility, label string, roller dice.Roller) (protocol.ChatMessage, error) {
	switch visibility {
	case visPublic, visGM, visSelf, visBlind:
	default:
		return protocol.ChatMessage{}, invalid("visibility must be public, gm, self or blind")
	}
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > maxLabelRunes {
		return protocol.ChatMessage{}, invalid("label must be at most %d characters", maxLabelRunes)
	}
	f, err := dice.Parse(formula)
	if err != nil {
		return protocol.ChatMessage{}, err
	}
	res := f.Roll(roller)
	roll := protocol.ChatRoll{Formula: res.Formula, Label: label, Result: &res}
	payload, err := json.Marshal(roll)
	if err != nil {
		return protocol.ChatMessage{}, err
	}
	m := protocol.ChatMessage{AuthorPlayerID: &authorID, Kind: kindRoll, Visibility: visibility, Roll: &roll}
	return s.insert(ctx, m, campaignID, gameSessionID, payload)
}

// insert completes m with an id and a time and stores it. The message it returns is complete.
func (s *Service) insert(ctx context.Context, m protocol.ChatMessage, campaignID, gameSessionID string, payload []byte) (protocol.ChatMessage, error) {
	m.ID = id.New()
	m.CreatedAt = s.now().UnixMilli()
	recipients := m.To
	if recipients == nil {
		recipients = []string{}
	}
	rec, err := json.Marshal(recipients)
	if err != nil {
		return protocol.ChatMessage{}, err
	}
	var gs any
	if gameSessionID != "" {
		gs = gameSessionID
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO chat_message (id, campaign_id, game_session_id, author_player_id, kind, visibility, recipients, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, campaignID, gs, m.AuthorPlayerID, m.Kind, m.Visibility, string(rec), string(payload), m.CreatedAt)
	if err != nil {
		return protocol.ChatMessage{}, fmt.Errorf("insert chat message: %w", err)
	}
	return m, nil
}

// History returns up to limit messages visible to v with an id below before,
// ascending. before must be a message of the campaign; limit 0 means 50.
func (s *Service) History(ctx context.Context, v ws.Viewer, campaignID, before string, limit int) ([]protocol.ChatMessage, error) {
	if limit < 0 || limit > maxHistory {
		return nil, invalid("limit must be 0..%d", maxHistory)
	}
	if limit == 0 {
		limit = snapshotLimit
	}
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM chat_message WHERE id = ? AND campaign_id = ?`, before, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find chat message: %w", err)
	}
	return s.visible(ctx, v, campaignID, before, limit)
}

// visibleQuery selects the messages visible to a viewer, newest first: ?1 campaign,
// ?2 the id to go before (empty: none), ?3 the viewer's player id (empty: none), ?4 1 for the GM, ?5 limit.
const visibleQuery = `SELECT id, created_at, author_player_id, kind, visibility, recipients, payload
 FROM chat_message
 WHERE campaign_id = ?1
   AND (?2 = '' OR id < ?2)
   AND ( visibility = 'public'
      OR author_player_id = ?3
      OR (visibility = 'private' AND EXISTS (SELECT 1 FROM json_each(recipients) WHERE value = ?3))
      OR (?4 = 1 AND visibility IN ('gm', 'blind')) )
 ORDER BY id DESC LIMIT ?5`

// visible reads the newest limit messages visible to v, ascending; with before
// set, only those with a smaller id. The filter is in SQL so that a page is
// not emptied by messages the viewer must not see; Render stays the authority.
func (s *Service) visible(ctx context.Context, v ws.Viewer, campaignID, before string, limit int) ([]protocol.ChatMessage, error) {
	gm := 0
	if v.IsGM {
		gm = 1
	}
	rows, err := s.db.QueryContext(ctx, visibleQuery, campaignID, before, v.PlayerID, gm, limit)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []protocol.ChatMessage{}
	for rows.Next() {
		var (
			m                   protocol.ChatMessage
			author              sql.NullString
			recipients, payload string
		)
		if err := rows.Scan(&m.ID, &m.CreatedAt, &author, &m.Kind, &m.Visibility, &recipients, &payload); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		if author.Valid {
			m.AuthorPlayerID = &author.String
		}
		var to []string
		if err := json.Unmarshal([]byte(recipients), &to); err != nil {
			return nil, fmt.Errorf("decode recipients of %s: %w", m.ID, err)
		}
		if len(to) > 0 {
			m.To = to
		}
		if err := decodePayload(&m, payload); err != nil {
			return nil, err
		}
		if r, ok := Render(m, v); ok {
			out = append(out, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	slices.Reverse(out)
	return out, nil
}

// decodePayload fills the text or the roll of m from its stored payload.
func decodePayload(m *protocol.ChatMessage, raw string) error {
	if m.Kind == kindRoll {
		var roll protocol.ChatRoll
		if err := json.Unmarshal([]byte(raw), &roll); err != nil {
			return fmt.Errorf("decode roll of %s: %w", m.ID, err)
		}
		m.Roll = &roll
		return nil
	}
	var p struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return fmt.Errorf("decode text of %s: %w", m.ID, err)
	}
	m.Text = p.Text
	return nil
}
