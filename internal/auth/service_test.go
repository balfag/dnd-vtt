package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/store"
)

type fixture struct {
	svc   *Service
	db    *sql.DB
	clock time.Time
}

// newFixture opens a migrated database and a Service driven by a fake clock
// that tests move through f.clock.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	f := &fixture{db: st.DB(), clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	f.svc, err = NewService(f.db, Options{Now: func() time.Time { return f.clock }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// createGM runs the whole first-run flow and returns the new GM account.
func (f *fixture) createGM(t *testing.T) Account {
	t.Helper()
	token, err := f.svc.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	acc, err := f.svc.CreateGM(context.Background(), token, "Master", "master-password")
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

// insertPlayer adds a non-GM account directly: the service has no API for it yet.
func (f *fixture) insertPlayer(t *testing.T, login, password string) Account {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	acc := Account{ID: "player-" + login, Login: login}
	_, err = f.db.Exec(
		`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, 0, 1, 1)`,
		acc.ID, acc.Login, hash)
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestFirstRun(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if exists, err := f.svc.GMExists(ctx); err != nil || exists {
		t.Fatalf("GMExists = %v, %v; want false", exists, err)
	}

	token, err := f.svc.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateGM(ctx, "wrong-token", "master", "master-password"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong token: err = %v, want ErrInvalidToken", err)
	}

	// Invalid input must not consume the token.
	if _, err := f.svc.CreateGM(ctx, token, "a b", "master-password"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("bad login: err = %v, want ErrInvalidLogin", err)
	}
	if _, err := f.svc.CreateGM(ctx, token, "master", "short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password: err = %v, want ErrInvalidPassword", err)
	}

	acc, err := f.svc.CreateGM(ctx, token, "Master", "master-password")
	if err != nil {
		t.Fatal(err)
	}
	if !acc.IsGM || acc.Login != "master" || acc.ID == "" {
		t.Errorf("account = %+v, want GM with login master", acc)
	}
	if exists, err := f.svc.GMExists(ctx); err != nil || !exists {
		t.Errorf("GMExists = %v, %v; want true", exists, err)
	}

	if _, err := f.svc.CreateGM(ctx, token, "other", "master-password"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("reused token: err = %v, want ErrInvalidToken", err)
	}
	second, err := f.svc.IssueSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateGM(ctx, second, "other", "master-password"); !errors.Is(err, ErrGMExists) {
		t.Errorf("second GM: err = %v, want ErrGMExists", err)
	}
}

func TestGMToken(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	early, err := f.svc.IssueGMToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RedeemGMToken(ctx, early); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("before GM exists: err = %v, want ErrInvalidToken", err)
	}

	gm := f.createGM(t)

	token, err := f.svc.IssueGMToken()
	if err != nil {
		t.Fatal(err)
	}
	acc, err := f.svc.RedeemGMToken(ctx, token)
	if err != nil || acc.ID != gm.ID || !acc.IsGM {
		t.Fatalf("redeem = %+v, %v; want the GM account", acc, err)
	}
	if _, err := f.svc.RedeemGMToken(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("second redeem: err = %v, want ErrInvalidToken", err)
	}

	expired, err := f.svc.IssueGMToken()
	if err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(GMTokenTTL)
	if _, err := f.svc.RedeemGMToken(ctx, expired); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired token: err = %v, want ErrInvalidToken", err)
	}

	almost, err := f.svc.IssueGMToken()
	if err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(GMTokenTTL - time.Second)
	if _, err := f.svc.RedeemGMToken(ctx, almost); err != nil {
		t.Errorf("token one second before expiry: err = %v, want nil", err)
	}

	if _, err := f.svc.RedeemGMToken(ctx, "garbage"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("garbage token: err = %v, want ErrInvalidToken", err)
	}
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)

	acc, err := f.svc.Authenticate(ctx, "master", "master-password")
	if err != nil || acc.ID != gm.ID {
		t.Fatalf("right credentials = %+v, %v", acc, err)
	}
	if _, err := f.svc.Authenticate(ctx, "MASTER", "master-password"); err != nil {
		t.Errorf("upper-case login: err = %v, want nil", err)
	}
	for _, tt := range []struct{ name, login, password string }{
		{"wrong password", "master", "not-the-password"},
		{"unknown login", "nobody", "master-password"},
		{"malformed login", "a b", "master-password"},
	} {
		if _, err := f.svc.Authenticate(ctx, tt.login, tt.password); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", tt.name, err)
		}
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)

	token, sess, err := f.svc.CreateSession(ctx, gm.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 {
		t.Errorf("token length = %d, want 43", len(token))
	}
	acc, got, err := f.svc.ResolveSession(ctx, token)
	if err != nil || acc.ID != gm.ID || got.ID != sess.ID || got.AccountID != gm.ID {
		t.Fatalf("resolve = %+v, %+v, %v", acc, got, err)
	}

	var stored string
	if err := f.db.QueryRow(`SELECT token_hash FROM login_session WHERE id = ?`, sess.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != HashToken(token) || stored == token {
		t.Errorf("stored token_hash = %q, want HashToken(token)", stored)
	}

	if _, _, err := f.svc.ResolveSession(ctx, "unknown"); !errors.Is(err, ErrNoSession) {
		t.Errorf("unknown token: err = %v, want ErrNoSession", err)
	}

	f.clock = f.clock.Add(time.Hour)
	if _, _, err := f.svc.ResolveSession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("expired: err = %v, want ErrNoSession", err)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM login_session WHERE id = ?`, sess.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("expired row count = %d, %v; want 0", n, err)
	}
}

func TestSessionRenewal(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)
	player := f.insertPlayer(t, "bob", "bob-password")

	gmToken, gmSess, err := f.svc.CreateSession(ctx, gm.ID, GMSessionTTL)
	if err != nil {
		t.Fatal(err)
	}
	start := f.clock

	f.clock = start.Add(30 * time.Minute)
	if _, s, err := f.svc.ResolveSession(ctx, gmToken); err != nil || !s.ExpiresAt.Equal(gmSess.ExpiresAt) {
		t.Fatalf("after 30m: expires = %v, %v; want unchanged %v", s.ExpiresAt, err, gmSess.ExpiresAt)
	}

	f.clock = start.Add(time.Hour)
	_, s, err := f.svc.ResolveSession(ctx, gmToken)
	if err != nil {
		t.Fatal(err)
	}
	if want := f.clock.Add(GMSessionTTL); !s.ExpiresAt.Equal(want) {
		t.Errorf("after 1h: expires = %v, want %v", s.ExpiresAt, want)
	}

	// A player's session is never extended.
	f.clock = start
	playerToken, playerSess, err := f.svc.CreateSession(ctx, player.ID, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.clock = start.Add(time.Hour)
	if _, s, err := f.svc.ResolveSession(ctx, playerToken); err != nil || !s.ExpiresAt.Equal(playerSess.ExpiresAt) {
		t.Errorf("player after 1h: expires = %v, %v; want unchanged %v", s.ExpiresAt, err, playerSess.ExpiresAt)
	}
}

func TestDeleteSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)

	token, sess, err := f.svc.CreateSession(ctx, gm.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.ResolveSession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("after delete: err = %v, want ErrNoSession", err)
	}
	if err := f.svc.DeleteSession(ctx, "does-not-exist"); err != nil {
		t.Errorf("deleting a missing session: err = %v, want nil", err)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)

	var tokens, ids [3]string
	for i := range tokens {
		tok, sess, err := f.svc.CreateSession(ctx, gm.ID, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		tokens[i], ids[i] = tok, sess.ID
	}

	if err := f.svc.ChangePassword(ctx, gm.ID, "wrong-current", "new-password-1", ids[0]); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current: err = %v, want ErrInvalidCredentials", err)
	}
	if err := f.svc.ChangePassword(ctx, gm.ID, "master-password", "short", ids[0]); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short new password: err = %v, want ErrInvalidPassword", err)
	}
	for i, tok := range tokens {
		if _, _, err := f.svc.ResolveSession(ctx, tok); err != nil {
			t.Fatalf("session %d must survive failed changes: %v", i, err)
		}
	}

	if err := f.svc.ChangePassword(ctx, gm.ID, "master-password", "new-password-1", ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(ctx, "master", "master-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("old password: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := f.svc.Authenticate(ctx, "master", "new-password-1"); err != nil {
		t.Errorf("new password: err = %v, want nil", err)
	}
	if _, _, err := f.svc.ResolveSession(ctx, tokens[0]); err != nil {
		t.Errorf("kept session: err = %v, want nil", err)
	}
	for _, tok := range tokens[1:] {
		if _, _, err := f.svc.ResolveSession(ctx, tok); !errors.Is(err, ErrNoSession) {
			t.Errorf("other session: err = %v, want ErrNoSession", err)
		}
	}
}

func TestGMTokensConcurrent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.createGM(t)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			token, err := f.svc.IssueGMToken()
			if err != nil {
				errs <- err
				return
			}
			if _, err := f.svc.RedeemGMToken(ctx, token); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// insertGameSession adds a campaign and an active game session by SQL: the
// auth package must not depend on game.
func (f *fixture) insertGameSession(t *testing.T) string {
	t.Helper()
	const gsID = "game-1"
	if _, err := f.db.Exec(`INSERT INTO campaign (id, name, created_at) VALUES ('camp-1', 'Camp', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO game_session (id, campaign_id, link_hash, started_at) VALUES (?, 'camp-1', ?, 1)`,
		gsID, HashToken("link")); err != nil {
		t.Fatal(err)
	}
	return gsID
}

func TestPlayerSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	player := f.insertPlayer(t, "bob", "bob-password")
	gsID := f.insertGameSession(t)

	token, sess, err := f.svc.CreatePlayerSession(ctx, player.ID, gsID)
	if err != nil {
		t.Fatal(err)
	}
	acc, got, err := f.svc.ResolveSession(ctx, token)
	if err != nil || acc.ID != player.ID {
		t.Fatalf("resolve = %+v, %v", acc, err)
	}
	if got.GameSessionID != gsID || got.ID != sess.ID {
		t.Errorf("session = %+v, want game session %s", got, gsID)
	}
	if want := f.clock.Add(PlayerSessionTTL); !got.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want)
	}
}

func TestPlayerSessionDiesWithGameSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	player := f.insertPlayer(t, "bob", "bob-password")
	gsID := f.insertGameSession(t)
	token, sess, err := f.svc.CreatePlayerSession(ctx, player.ID, gsID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.db.Exec(`UPDATE game_session SET ended_at = ? WHERE id = ?`, f.clock.UnixMilli(), gsID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.ResolveSession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after the game ended: err = %v, want ErrNoSession", err)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM login_session WHERE id = ?`, sess.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("session rows = %d, %v; want 0", n, err)
	}
}

func TestPlayerSessionExpires(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	player := f.insertPlayer(t, "bob", "bob-password")
	gsID := f.insertGameSession(t)
	token, _, err := f.svc.CreatePlayerSession(ctx, player.ID, gsID)
	if err != nil {
		t.Fatal(err)
	}

	f.clock = f.clock.Add(PlayerSessionTTL)
	if _, _, err := f.svc.ResolveSession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Errorf("after the TTL: err = %v, want ErrNoSession", err)
	}
}

func TestGMSessionHasNoGameSession(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gm := f.createGM(t)

	token, _, err := f.svc.CreateSession(ctx, gm.ID, GMSessionTTL)
	if err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(time.Hour)
	_, sess, err := f.svc.ResolveSession(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if sess.GameSessionID != "" {
		t.Errorf("GameSessionID = %q, want empty", sess.GameSessionID)
	}
	if want := f.clock.Add(GMSessionTTL); !sess.ExpiresAt.Equal(want) {
		t.Errorf("GM session was not renewed: ExpiresAt = %v, want %v", sess.ExpiresAt, want)
	}
}
