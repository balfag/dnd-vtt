package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "dndvtt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenNonASCIIPath(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "Мастер Игры", "data dir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dndvtt.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file missing: %v", err)
	}
	if v, err := s.SchemaVersion(ctx); err != nil || v != 3 {
		t.Errorf("SchemaVersion = %d, %v; want 3", v, err)
	}
}

func TestPragmasOnEveryConnection(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)

	for i := range 3 {
		// Connections are intentionally held open together so the pool has to
		// create three distinct ones.
		conn, err := s.DB().Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()

		var fk int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
			t.Errorf("conn %d: foreign_keys = %d, %v; want 1", i, fk, err)
		}
		var mode string
		if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
			t.Errorf("conn %d: journal_mode = %q, %v; want wal", i, mode, err)
		}
	}
}

func TestReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dndvtt.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if v, err := s.SchemaVersion(ctx); err != nil || v != 3 {
		t.Errorf("SchemaVersion = %d, %v; want 3", v, err)
	}
	if n := countMigrations(t, s.DB()); n != 3 {
		t.Errorf("schema_migrations rows = %d, want 3", n)
	}
}

func TestOpenMissingDirectory(t *testing.T) {
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "missing", "x.db"))
	if err == nil {
		_ = s.Close()
		t.Fatal("Open succeeded in a missing directory, want error")
	}
}

func TestCampaignSchema(t *testing.T) {
	s := openStore(t)

	tests := []struct {
		name    string
		query   string
		args    []any
		wantErr bool
	}{
		{"valid", `INSERT INTO campaign (id, name, created_at) VALUES ('c1', 'Кампания', 1)`, nil, false},
		{"empty name", `INSERT INTO campaign (id, name, created_at) VALUES ('c2', '', 1)`, nil, true},
		{"name too long", `INSERT INTO campaign (id, name, created_at) VALUES ('c3', ?, 1)`, []any{strings.Repeat("x", 201)}, true},
		{"created_at not integer", `INSERT INTO campaign (id, name, created_at) VALUES ('c4', 'n', 'abc')`, nil, true},
		{"settings not json", `INSERT INTO campaign (id, name, settings, created_at) VALUES ('c5', 'n', 'not json', 1)`, nil, true},
		{"duplicate id", `INSERT INTO campaign (id, name, created_at) VALUES ('c1', 'other', 2)`, nil, true},
	}
	// Subtests run in order: "duplicate id" relies on "valid" having run.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.DB().Exec(tt.query, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	var settings string
	if err := s.DB().QueryRow(`SELECT settings FROM campaign WHERE id = 'c1'`).Scan(&settings); err != nil {
		t.Fatal(err)
	}
	if settings != "{}" {
		t.Errorf("default settings = %q, want {}", settings)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	s := openStore(t)
	db := s.DB()

	for _, q := range []string{
		`CREATE TABLE fk_parent (id TEXT PRIMARY KEY) STRICT`,
		`CREATE TABLE fk_child (parent_id TEXT NOT NULL REFERENCES fk_parent (id)) STRICT`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_, err := db.Exec(`INSERT INTO fk_child (parent_id) VALUES ('nope')`)
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("err = %v, want FOREIGN KEY violation", err)
	}
}

func TestAccountSchema(t *testing.T) {
	s := openStore(t)
	db := s.DB()

	const hash = "$argon2id$v=19$x"
	insert := func(id, login, hash string, isGM int) error {
		_, err := db.Exec(
			`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES (?, ?, ?, ?, 1, 1)`,
			id, login, hash, isGM)
		return err
	}

	tests := []struct {
		name    string
		id      string
		login   string
		hash    string
		isGM    int
		wantErr bool
	}{
		{"gm", "a1", "anna", hash, 1, false},
		{"player with punctuation", "a2", "bob.smith-1_x", hash, 0, false},
		{"uppercase login", "a3", "Anna2", hash, 0, true},
		{"too short login", "a4", "ab", hash, 0, true},
		{"login with space", "a5", "bad login", hash, 0, true},
		{"non-ascii login", "a6", "юзер", hash, 0, true},
		{"login of 33 chars", "a7", strings.Repeat("a", 33), hash, 0, true},
		{"wrong hash algorithm", "a8", "carol", "$argon2i$v=19$x", 0, true},
		{"second gm", "a9", "dave", hash, 1, true},
		{"duplicate login", "a10", "anna", hash, 0, true},
	}
	// Subtests run in order: later ones rely on the rows inserted by earlier ones.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := insert(tt.id, tt.login, tt.hash, tt.isGM)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("session of unknown account", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO login_session (id, account_id, token_hash, created_at, last_seen_at, expires_at)
			VALUES ('s0', 'nope', ?, 1, 1, 2)`, strings.Repeat("a", 64))
		if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Fatalf("err = %v, want FOREIGN KEY violation", err)
		}
	})

	t.Run("deleting account cascades to sessions", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO login_session (id, account_id, token_hash, created_at, last_seen_at, expires_at)
			VALUES ('s1', 'a2', ?, 1, 1, 2)`, strings.Repeat("b", 64)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM account WHERE id = 'a2'`); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM login_session`).Scan(&n); err != nil || n != 0 {
			t.Errorf("sessions left = %d, %v; want 0", n, err)
		}
	})
}

func TestGameSessionSchema(t *testing.T) {
	s := openStore(t)
	db := s.DB()
	exec := func(q string, args ...any) error {
		_, err := db.Exec(q, args...)
		return err
	}
	must := func(q string, args ...any) {
		t.Helper()
		if err := exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	hash := func(c string) string { return strings.Repeat(c, 64) }

	must(`INSERT INTO campaign (id, name, created_at) VALUES ('c1', 'Camp', 1)`)
	must(`INSERT INTO account (id, login, password_hash, is_gm, created_at, password_changed_at) VALUES ('a1', 'anna', '$argon2id$x', 0, 1, 1)`)

	t.Run("one active game session", func(t *testing.T) {
		must(`INSERT INTO game_session (id, campaign_id, link_hash, started_at) VALUES ('g1', 'c1', ?, 1)`, hash("a"))
		if err := exec(`INSERT INTO game_session (id, campaign_id, link_hash, started_at) VALUES ('g2', 'c1', ?, 2)`, hash("b")); err == nil {
			t.Fatal("a second active game session was accepted")
		}
		must(`UPDATE game_session SET ended_at = 5 WHERE id = 'g1'`)
		must(`INSERT INTO game_session (id, campaign_id, link_hash, started_at) VALUES ('g2', 'c1', ?, 6)`, hash("b"))
		must(`UPDATE game_session SET ended_at = 7 WHERE id = 'g2'`)
		must(`INSERT INTO game_session (id, campaign_id, link_hash, started_at, ended_at) VALUES ('g3', 'c1', ?, 8, 9)`, hash("c"))
		must(`INSERT INTO game_session (id, campaign_id, link_hash, started_at, ended_at) VALUES ('g4', 'c1', ?, 8, 9)`, hash("d"))
	})

	t.Run("login session binding", func(t *testing.T) {
		err := exec(`INSERT INTO login_session (id, account_id, token_hash, created_at, last_seen_at, expires_at, game_session_id)
			VALUES ('s0', 'a1', ?, 1, 1, 2, 'nope')`, hash("e"))
		if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Fatalf("err = %v, want FOREIGN KEY violation", err)
		}
		must(`INSERT INTO login_session (id, account_id, token_hash, created_at, last_seen_at, expires_at, game_session_id)
			VALUES ('s1', 'a1', ?, 1, 1, 2, 'g3')`, hash("f"))
		must(`DELETE FROM game_session WHERE id = 'g3'`)
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM login_session`).Scan(&n); err != nil || n != 0 {
			t.Errorf("sessions left = %d, %v; want 0", n, err)
		}
	})

	t.Run("player constraints", func(t *testing.T) {
		const q = `INSERT INTO player (id, campaign_id, account_id, display_name, color, role, created_at) VALUES (?, 'c1', 'a1', 'Anna', ?, ?, 1)`
		if err := exec(q, "p1", "#e53935", "spectator"); err == nil {
			t.Error("role spectator was accepted")
		}
		if err := exec(q, "p1", "#E53935", "player"); err == nil {
			t.Error("upper-case color was accepted")
		}
		must(q, "p1", "#e53935", "player")
		if err := exec(q, "p2", "#e53935", "player"); err == nil {
			t.Error("duplicate (campaign_id, account_id) was accepted")
		}
	})
}
