// Package game implements campaigns, game sessions and campaign participants (ADR-0008).
package game

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/id"
)

var (
	ErrNotFound       = errors.New("game: not found")
	ErrNoActiveGame   = errors.New("game: no active game session")
	ErrRemoved        = errors.New("game: player was removed by the gm")
	ErrInvalidName    = errors.New("game: invalid name")
	ErrLoginTaken     = errors.New("game: login is already taken")
	ErrCannotRemoveGM = errors.New("game: the gm cannot be removed")
)

// Roles of a player in a campaign.
const (
	RoleGM     = "gm"
	RolePlayer = "player"
)

const (
	maxCampaignNameLen = 200
	maxDisplayNameLen  = 64
	devCampaignName    = "Dev campaign"
)

// playerColors are assigned in order of joining a campaign, wrapping around.
var playerColors = []string{
	"#e53935", "#1e88e5", "#43a047", "#fb8c00",
	"#8e24aa", "#00acc1", "#fdd835", "#6d4c41",
}

type Campaign struct {
	ID                string
	Name              string
	CreatedAt         time.Time
	ActiveGameSession *GameSession // nil when this campaign has no active game
}

type GameSession struct {
	ID         string
	CampaignID string
	StartedAt  time.Time
}

type Player struct {
	ID           string
	CampaignID   string
	AccountID    string
	AccountLogin string
	DisplayName  string
	Color        string
	Role         string // RoleGM or RolePlayer
	Removed      bool
	CreatedAt    time.Time
}

type Options struct {
	Now func() time.Time // nil means time.Now
}

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

// querier is what both *sql.DB and *sql.Tx offer.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// validName trims name and checks its length in runes against 1..max.
func validName(name string, max int) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > max {
		return "", ErrInvalidName
	}
	return name, nil
}

// --- Campaigns ---

const campaignSelect = `
	SELECT c.id, c.name, c.created_at, gs.id, gs.started_at
	FROM campaign c
	LEFT JOIN game_session gs ON gs.campaign_id = c.id AND gs.ended_at IS NULL`

type rowScanner interface{ Scan(dest ...any) error }

func scanCampaign(r rowScanner) (Campaign, error) {
	var (
		c         Campaign
		createdMs int64
		gsID      sql.NullString
		gsStarted sql.NullInt64
	)
	if err := r.Scan(&c.ID, &c.Name, &createdMs, &gsID, &gsStarted); err != nil {
		return Campaign{}, err
	}
	c.CreatedAt = time.UnixMilli(createdMs)
	if gsID.Valid {
		c.ActiveGameSession = &GameSession{ID: gsID.String, CampaignID: c.ID, StartedAt: time.UnixMilli(gsStarted.Int64)}
	}
	return c, nil
}

func (s *Service) CreateCampaign(ctx context.Context, name string) (Campaign, error) {
	name, err := validName(name, maxCampaignNameLen)
	if err != nil {
		return Campaign{}, err
	}
	c := Campaign{ID: id.New(), Name: name, CreatedAt: s.now()}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO campaign (id, name, created_at) VALUES (?, ?, ?)`,
		c.ID, c.Name, c.CreatedAt.UnixMilli()); err != nil {
		return Campaign{}, fmt.Errorf("create campaign: %w", err)
	}
	return c, nil
}

func (s *Service) GetCampaign(ctx context.Context, id string) (Campaign, error) {
	c, err := scanCampaign(s.db.QueryRowContext(ctx, campaignSelect+` WHERE c.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	if err != nil {
		return Campaign{}, fmt.Errorf("get campaign: %w", err)
	}
	return c, nil
}

func (s *Service) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.db.QueryContext(ctx, campaignSelect+` ORDER BY c.created_at, c.id`)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	return out, nil
}

func (s *Service) RenameCampaign(ctx context.Context, id, name string) (Campaign, error) {
	name, err := validName(name, maxCampaignNameLen)
	if err != nil {
		return Campaign{}, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE campaign SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return Campaign{}, fmt.Errorf("rename campaign: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Campaign{}, fmt.Errorf("rename campaign: %w", err)
	} else if n == 0 {
		return Campaign{}, ErrNotFound
	}
	return s.GetCampaign(ctx, id)
}

// EnsureDevCampaign creates "Dev campaign" if there are no campaigns at all
// and reports whether it did. Used only in -dev mode.
func (s *Service) EnsureDevCampaign(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign`).Scan(&n); err != nil {
		return false, fmt.Errorf("count campaigns: %w", err)
	}
	if n > 0 {
		return false, nil
	}
	if _, err := s.CreateCampaign(ctx, devCampaignName); err != nil {
		return false, err
	}
	return true, nil
}

// --- Game sessions ---

// StartGameSession ends the active game session (if any, in any campaign)
// together with its player login sessions, starts a new one in campaignID,
// makes sure gm is a participant of the campaign with RoleGM and returns the
// new session and its link token (shown once; only the hash is stored).
func (s *Service) StartGameSession(ctx context.Context, campaignID string, gm auth.Account) (GameSession, string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return GameSession{}, "", err
	}
	now := s.now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GameSession{}, "", fmt.Errorf("start game session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM campaign WHERE id = ?`, campaignID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return GameSession{}, "", ErrNotFound
	}
	if err != nil {
		return GameSession{}, "", fmt.Errorf("start game session: find campaign: %w", err)
	}

	var activeID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM game_session WHERE ended_at IS NULL`).Scan(&activeID)
	switch {
	case err == nil:
		if err := endGameSession(ctx, tx, activeID, now); err != nil {
			return GameSession{}, "", fmt.Errorf("start game session: end previous: %w", err)
		}
	case !errors.Is(err, sql.ErrNoRows):
		return GameSession{}, "", fmt.Errorf("start game session: find active: %w", err)
	}

	gs := GameSession{ID: id.New(), CampaignID: campaignID, StartedAt: now}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO game_session (id, campaign_id, link_hash, started_at) VALUES (?, ?, ?, ?)`,
		gs.ID, campaignID, auth.HashToken(token), now.UnixMilli()); err != nil {
		return GameSession{}, "", fmt.Errorf("start game session: insert: %w", err)
	}

	n, err := countPlayers(ctx, tx, campaignID)
	if err != nil {
		return GameSession{}, "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO player (id, campaign_id, account_id, display_name, color, role, created_at)
		VALUES (?, ?, ?, ?, ?, 'gm', ?)
		ON CONFLICT (campaign_id, account_id) DO NOTHING`,
		id.New(), campaignID, gm.ID, gm.Login, playerColors[n%len(playerColors)], now.UnixMilli()); err != nil {
		return GameSession{}, "", fmt.Errorf("start game session: add gm: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return GameSession{}, "", fmt.Errorf("start game session: commit: %w", err)
	}
	return gs, token, nil
}

// endGameSession marks an active game session ended and drops the login
// sessions bound to it.
func endGameSession(ctx context.Context, q querier, gameSessionID string, now time.Time) error {
	if _, err := q.ExecContext(ctx,
		`UPDATE game_session SET ended_at = ? WHERE id = ? AND ended_at IS NULL`,
		now.UnixMilli(), gameSessionID); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `DELETE FROM login_session WHERE game_session_id = ?`, gameSessionID)
	return err
}

func (s *Service) EndGameSession(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("end game session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE game_session SET ended_at = ? WHERE id = ? AND ended_at IS NULL`,
		s.now().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("end game session: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("end game session: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_session WHERE game_session_id = ?`, id); err != nil {
		return fmt.Errorf("end game session: drop login sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("end game session: commit: %w", err)
	}
	return nil
}

func (s *Service) RotateLink(ctx context.Context, id string) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE game_session SET link_hash = ? WHERE id = ? AND ended_at IS NULL`,
		auth.HashToken(token), id)
	if err != nil {
		return "", fmt.Errorf("rotate link: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", fmt.Errorf("rotate link: %w", err)
	} else if n == 0 {
		return "", ErrNotFound
	}
	return token, nil
}

func scanGameSession(r rowScanner) (GameSession, error) {
	var (
		gs        GameSession
		startedMs int64
	)
	if err := r.Scan(&gs.ID, &gs.CampaignID, &startedMs); err != nil {
		return GameSession{}, err
	}
	gs.StartedAt = time.UnixMilli(startedMs)
	return gs, nil
}

func (s *Service) ActiveGameSession(ctx context.Context) (GameSession, error) {
	gs, err := scanGameSession(s.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, started_at FROM game_session WHERE ended_at IS NULL`))
	if errors.Is(err, sql.ErrNoRows) {
		return GameSession{}, ErrNoActiveGame
	}
	if err != nil {
		return GameSession{}, fmt.Errorf("active game session: %w", err)
	}
	return gs, nil
}

// ResolveLink returns the active game session whose link token is token.
func (s *Service) ResolveLink(ctx context.Context, token string) (GameSession, error) {
	gs, err := scanGameSession(s.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, started_at FROM game_session WHERE link_hash = ? AND ended_at IS NULL`,
		auth.HashToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return GameSession{}, ErrNoActiveGame
	}
	if err != nil {
		return GameSession{}, fmt.Errorf("resolve link: %w", err)
	}
	return gs, nil
}

// --- Players ---

const playerSelect = `
	SELECT p.id, p.campaign_id, p.account_id, a.login, p.display_name, p.color, p.role, p.removed, p.created_at
	FROM player p JOIN account a ON a.id = p.account_id`

func scanPlayer(r rowScanner) (Player, error) {
	var (
		p         Player
		removed   int
		createdMs int64
	)
	if err := r.Scan(&p.ID, &p.CampaignID, &p.AccountID, &p.AccountLogin, &p.DisplayName,
		&p.Color, &p.Role, &removed, &createdMs); err != nil {
		return Player{}, err
	}
	p.Removed = removed == 1
	p.CreatedAt = time.UnixMilli(createdMs)
	return p, nil
}

func countPlayers(ctx context.Context, q querier, campaignID string) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM player WHERE campaign_id = ?`, campaignID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count players: %w", err)
	}
	return n, nil
}

// insertPlayer adds a participant with the next color of the campaign.
func (s *Service) insertPlayer(ctx context.Context, q querier, campaignID, accountID, login, displayName, role string) (Player, error) {
	n, err := countPlayers(ctx, q, campaignID)
	if err != nil {
		return Player{}, err
	}
	p := Player{
		ID:           id.New(),
		CampaignID:   campaignID,
		AccountID:    accountID,
		AccountLogin: login,
		DisplayName:  displayName,
		Color:        playerColors[n%len(playerColors)],
		Role:         role,
		CreatedAt:    s.now(),
	}
	if _, err := q.ExecContext(ctx,
		`INSERT INTO player (id, campaign_id, account_id, display_name, color, role, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CampaignID, p.AccountID, p.DisplayName, p.Color, p.Role, p.CreatedAt.UnixMilli()); err != nil {
		return Player{}, fmt.Errorf("insert player: %w", err)
	}
	return p, nil
}

// requireActive reports ErrNoActiveGame unless gameSessionID is still active.
func requireActive(ctx context.Context, q querier, gameSessionID string) error {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM game_session WHERE id = ? AND ended_at IS NULL`, gameSessionID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNoActiveGame
	}
	if err != nil {
		return fmt.Errorf("check game session: %w", err)
	}
	return nil
}

// RegisterPlayer creates a non-GM account and its participant in the
// campaign of gs, in one transaction.
func (s *Service) RegisterPlayer(ctx context.Context, gs GameSession, login, password, displayName string) (auth.Account, Player, error) {
	login, err := auth.NormalizeLogin(login)
	if err != nil {
		return auth.Account{}, Player{}, err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return auth.Account{}, Player{}, err
	}
	displayName, err = validName(displayName, maxDisplayNameLen)
	if err != nil {
		return auth.Account{}, Player{}, err
	}
	// Hash before the transaction: it is slow and would hold the write lock.
	hash, err := auth.HashPassword(password)
	if err != nil {
		return auth.Account{}, Player{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.Account{}, Player{}, fmt.Errorf("register player: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireActive(ctx, tx, gs.ID); err != nil {
		return auth.Account{}, Player{}, err
	}
	acc := auth.Account{ID: id.New(), Login: login}
	nowMs := s.now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, 0, ?, ?)`,
		acc.ID, acc.Login, hash, nowMs, nowMs); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: account.login") {
			return auth.Account{}, Player{}, ErrLoginTaken
		}
		return auth.Account{}, Player{}, fmt.Errorf("register player: insert account: %w", err)
	}
	p, err := s.insertPlayer(ctx, tx, gs.CampaignID, acc.ID, acc.Login, displayName, RolePlayer)
	if err != nil {
		return auth.Account{}, Player{}, fmt.Errorf("register player: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return auth.Account{}, Player{}, fmt.Errorf("register player: commit: %w", err)
	}
	return acc, p, nil
}

// JoinPlayer returns acc's participant in the campaign of gs, creating it
// (display name = acc.Login) when acc has not played there yet.
func (s *Service) JoinPlayer(ctx context.Context, gs GameSession, acc auth.Account) (Player, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Player{}, fmt.Errorf("join player: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireActive(ctx, tx, gs.ID); err != nil {
		return Player{}, err
	}
	p, err := scanPlayer(tx.QueryRowContext(ctx, playerSelect+` WHERE p.campaign_id = ? AND p.account_id = ?`, gs.CampaignID, acc.ID))
	switch {
	case err == nil:
		if p.Removed {
			return Player{}, ErrRemoved
		}
		return p, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Player{}, fmt.Errorf("join player: find: %w", err)
	}

	role := RolePlayer
	if acc.IsGM {
		role = RoleGM
	}
	p, err = s.insertPlayer(ctx, tx, gs.CampaignID, acc.ID, acc.Login, acc.Login, role)
	if err != nil {
		return Player{}, fmt.Errorf("join player: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Player{}, fmt.Errorf("join player: commit: %w", err)
	}
	return p, nil
}

func (s *Service) PlayerForAccount(ctx context.Context, campaignID, accountID string) (Player, error) {
	p, err := scanPlayer(s.db.QueryRowContext(ctx, playerSelect+` WHERE p.campaign_id = ? AND p.account_id = ?`, campaignID, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrNotFound
	}
	if err != nil {
		return Player{}, fmt.Errorf("player for account: %w", err)
	}
	return p, nil
}

func (s *Service) ListPlayers(ctx context.Context, campaignID string) ([]Player, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM campaign WHERE id = ?`, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("list players: find campaign: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, playerSelect+` WHERE p.campaign_id = ? ORDER BY p.created_at, p.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	return out, nil
}

// RemovePlayer marks the participant removed and deletes the account's login
// sessions bound to game sessions of that campaign.
func (s *Service) RemovePlayer(ctx context.Context, playerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("remove player: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var campaignID, accountID, role string
	err = tx.QueryRowContext(ctx, `SELECT campaign_id, account_id, role FROM player WHERE id = ?`, playerID).
		Scan(&campaignID, &accountID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove player: find: %w", err)
	}
	if role == RoleGM {
		return ErrCannotRemoveGM
	}

	if _, err := tx.ExecContext(ctx, `UPDATE player SET removed = 1 WHERE id = ?`, playerID); err != nil {
		return fmt.Errorf("remove player: mark: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM login_session
		WHERE account_id = ? AND game_session_id IN (SELECT id FROM game_session WHERE campaign_id = ?)`,
		accountID, campaignID); err != nil {
		return fmt.Errorf("remove player: drop login sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("remove player: commit: %w", err)
	}
	return nil
}
