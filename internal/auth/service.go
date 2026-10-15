package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/balfag/dnd-vtt/internal/id"
)

// Errors returned by Service. Callers compare with errors.Is.
var (
	ErrInvalidLogin       = errors.New("auth: invalid login")
	ErrInvalidPassword    = errors.New("auth: invalid password")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrInvalidToken       = errors.New("auth: invalid or expired token")
	ErrGMExists           = errors.New("auth: gm account already exists")
	ErrNoSession          = errors.New("auth: no valid login session")
)

const (
	// GMSessionTTL is how long a GM login session lives without use.
	GMSessionTTL = 30 * 24 * time.Hour
	// GMTokenTTL is how long a one-time GM sign-in token is valid.
	GMTokenTTL = 10 * time.Minute
	// sessionRenewInterval limits how often a GM session is extended.
	sessionRenewInterval = time.Hour
)

// Account is the public view of an account row.
type Account struct {
	ID    string
	Login string
	IsGM  bool
}

// Session is a login session on one device.
type Session struct {
	ID        string
	AccountID string
	ExpiresAt time.Time
}

// Options configures a Service. Zero value is valid.
type Options struct {
	// Now returns the current time; nil means time.Now. Tests pass a fake clock.
	Now func() time.Time
}

// Service manages accounts, one-time tokens and login sessions.
// It is safe for concurrent use.
type Service struct {
	db  *sql.DB
	now func() time.Time

	// mu guards the in-memory one-time tokens only; never hold it during
	// database calls.
	mu        sync.Mutex
	setupHash string               // HashToken of the first-run token, "" if none
	gmTokens  map[string]time.Time // HashToken -> expiry

	// dummyHash equalises timing for logins that do not exist.
	dummyHash string
}

// NewService returns a Service on db (already migrated). It precomputes a
// dummy password hash used to equalise timing for unknown logins.
func NewService(db *sql.DB, opts Options) (*Service, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	dummy, err := HashPassword("dummy password for timing")
	if err != nil {
		return nil, fmt.Errorf("precompute dummy hash: %w", err)
	}
	return &Service{db: db, now: now, gmTokens: make(map[string]time.Time), dummyHash: dummy}, nil
}

// GMExists reports whether the GM account has been created.
func (s *Service) GMExists(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM account WHERE is_gm = 1)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check gm account: %w", err)
	}
	return exists, nil
}

// IssueSetupToken creates the one-time first-run token, replacing any
// previous one. It stays valid until used or the process exits.
func (s *Service) IssueSetupToken() (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.setupHash = HashToken(token)
	s.mu.Unlock()
	return token, nil
}

// checkSetupToken reports whether token is the current setup token.
func (s *Service) checkSetupToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setupHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(HashToken(token)), []byte(s.setupHash)) == 1
}

// CreateGM consumes setupToken and creates the GM account.
// Errors: ErrInvalidToken, ErrGMExists, ErrInvalidLogin, ErrInvalidPassword.
func (s *Service) CreateGM(ctx context.Context, setupToken, login, password string) (Account, error) {
	if !s.checkSetupToken(setupToken) {
		return Account{}, ErrInvalidToken
	}
	exists, err := s.GMExists(ctx)
	if err != nil {
		return Account{}, err
	}
	if exists {
		return Account{}, ErrGMExists
	}
	login, err = NormalizeLogin(login)
	if err != nil {
		return Account{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return Account{}, err
	}
	// Hash before the transaction: it is slow and would hold the write lock.
	hash, err := HashPassword(password)
	if err != nil {
		return Account{}, err
	}

	acc := Account{ID: id.New(), Login: login, IsGM: true}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, fmt.Errorf("create gm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var gmCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM account WHERE is_gm = 1`).Scan(&gmCount); err != nil {
		return Account{}, fmt.Errorf("create gm: check existing: %w", err)
	}
	if gmCount > 0 {
		return Account{}, ErrGMExists
	}
	nowMs := s.now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, 1, ?, ?)`,
		acc.ID, acc.Login, hash, nowMs, nowMs); err != nil {
		return Account{}, fmt.Errorf("create gm: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Account{}, fmt.Errorf("create gm: commit: %w", err)
	}

	s.mu.Lock()
	s.setupHash = ""
	s.mu.Unlock()
	return acc, nil
}

// IssueGMToken creates a one-time GM sign-in token valid for GMTokenTTL.
// Several tokens may be valid at once; expired ones are dropped on each call.
func (s *Service) IssueGMToken() (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, exp := range s.gmTokens {
		if !now.Before(exp) {
			delete(s.gmTokens, h)
		}
	}
	s.gmTokens[HashToken(token)] = now.Add(GMTokenTTL)
	return token, nil
}

// RedeemGMToken consumes token and returns the GM account.
// Errors: ErrInvalidToken (unknown, used, expired, or no GM account yet).
func (s *Service) RedeemGMToken(ctx context.Context, token string) (Account, error) {
	h := HashToken(token)
	now := s.now()

	s.mu.Lock()
	exp, ok := s.gmTokens[h]
	delete(s.gmTokens, h) // one-time, whatever happens next
	s.mu.Unlock()
	if !ok || !now.Before(exp) {
		return Account{}, ErrInvalidToken
	}

	var acc Account
	err := s.db.QueryRowContext(ctx, `SELECT id, login, is_gm FROM account WHERE is_gm = 1`).
		Scan(&acc.ID, &acc.Login, &acc.IsGM)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrInvalidToken
	}
	if err != nil {
		return Account{}, fmt.Errorf("load gm account: %w", err)
	}
	return acc, nil
}

// Authenticate checks login and password. Any failure, including a login
// that does not pass NormalizeLogin, is ErrInvalidCredentials.
func (s *Service) Authenticate(ctx context.Context, login, password string) (Account, error) {
	normalized, err := NormalizeLogin(login)
	if err != nil {
		s.spendDummyHash(password)
		return Account{}, ErrInvalidCredentials
	}

	var (
		acc  Account
		hash string
	)
	err = s.db.QueryRowContext(ctx, `SELECT id, login, is_gm, password_hash FROM account WHERE login = ?`, normalized).
		Scan(&acc.ID, &acc.Login, &acc.IsGM, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		s.spendDummyHash(password)
		return Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return Account{}, fmt.Errorf("load account: %w", err)
	}

	ok, err := VerifyPassword(hash, password)
	if err != nil {
		return Account{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return Account{}, ErrInvalidCredentials
	}
	return acc, nil
}

// spendDummyHash burns the same time as a real password check.
func (s *Service) spendDummyHash(password string) {
	_, _ = VerifyPassword(s.dummyHash, password)
}

// CreateSession creates a login session for accountID that expires after ttl
// and returns its token (shown to the client once) and the session.
func (s *Service) CreateSession(ctx context.Context, accountID string, ttl time.Duration) (string, Session, error) {
	token, err := NewToken()
	if err != nil {
		return "", Session{}, err
	}
	now := s.now()
	sess := Session{ID: id.New(), AccountID: accountID, ExpiresAt: now.Add(ttl)}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO login_session (id, account_id, token_hash, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		sess.ID, accountID, HashToken(token), now.UnixMilli(), now.UnixMilli(), sess.ExpiresAt.UnixMilli()); err != nil {
		return "", Session{}, fmt.Errorf("create login session: %w", err)
	}
	return token, sess, nil
}

// ResolveSession returns the account and session for token.
// Errors: ErrNoSession (unknown or expired; an expired row is deleted).
// For the GM, a session last seen at least sessionRenewInterval ago is
// extended: last_seen_at = now, expires_at = now + GMSessionTTL.
func (s *Service) ResolveSession(ctx context.Context, token string) (Account, Session, error) {
	var (
		acc                   Account
		sess                  Session
		lastSeenMs, expiresMs int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.login, a.is_gm, ls.id, ls.last_seen_at, ls.expires_at
		FROM login_session ls JOIN account a ON a.id = ls.account_id
		WHERE ls.token_hash = ?`, HashToken(token)).
		Scan(&acc.ID, &acc.Login, &acc.IsGM, &sess.ID, &lastSeenMs, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, Session{}, ErrNoSession
	}
	if err != nil {
		return Account{}, Session{}, fmt.Errorf("load login session: %w", err)
	}
	sess.AccountID = acc.ID

	now := s.now()
	if expiresMs <= now.UnixMilli() {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM login_session WHERE id = ?`, sess.ID); err != nil {
			return Account{}, Session{}, fmt.Errorf("delete expired login session: %w", err)
		}
		return Account{}, Session{}, ErrNoSession
	}

	if acc.IsGM && now.Sub(time.UnixMilli(lastSeenMs)) >= sessionRenewInterval {
		expiresMs = now.Add(GMSessionTTL).UnixMilli()
		if _, err := s.db.ExecContext(ctx,
			`UPDATE login_session SET last_seen_at = ?, expires_at = ? WHERE id = ?`,
			now.UnixMilli(), expiresMs, sess.ID); err != nil {
			return Account{}, Session{}, fmt.Errorf("renew login session: %w", err)
		}
	}
	sess.ExpiresAt = time.UnixMilli(expiresMs)
	return acc, sess, nil
}

// DeleteSession removes a login session. Deleting a missing one is not an error.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM login_session WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("delete login session: %w", err)
	}
	return nil
}

// ChangePassword verifies current, sets newPassword and deletes every other
// login session of the account except keepSessionID.
// Errors: ErrInvalidCredentials (wrong current), ErrInvalidPassword (bad new).
func (s *Service) ChangePassword(ctx context.Context, accountID, current, newPassword, keepSessionID string) error {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM account WHERE id = ?`, accountID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		s.spendDummyHash(current)
		return ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	ok, err := VerifyPassword(hash, current)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return ErrInvalidCredentials
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("change password: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE account SET password_hash = ?, password_changed_at = ? WHERE id = ?`,
		newHash, s.now().UnixMilli(), accountID); err != nil {
		return fmt.Errorf("change password: update: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM login_session WHERE account_id = ? AND id <> ?`, accountID, keepSessionID); err != nil {
		return fmt.Errorf("change password: drop sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("change password: commit: %w", err)
	}
	return nil
}
