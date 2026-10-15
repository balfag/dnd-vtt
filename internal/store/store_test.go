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
	if v, err := s.SchemaVersion(ctx); err != nil || v != 1 {
		t.Errorf("SchemaVersion = %d, %v; want 1", v, err)
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
	if v, err := s.SchemaVersion(ctx); err != nil || v != 1 {
		t.Errorf("SchemaVersion = %d, %v; want 1", v, err)
	}
	if n := countMigrations(t, s.DB()); n != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", n)
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
