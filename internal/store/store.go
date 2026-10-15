// Package store owns the SQLite database: it opens it with the required
// pragmas and brings the schema up to date with embedded migrations.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store is the application's handle to the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creating the file if needed,
// checks that WAL and foreign keys are enabled and applies pending
// migrations. The parent directory must exist.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	// An open handle keeps the file locked on Windows, so every failure
	// below must release it.
	if err := prepare(ctx, db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func prepare(ctx context.Context, db *sql.DB, path string) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database %s: %w", path, err)
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		return fmt.Errorf("read journal_mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("database %s: journal_mode is %q, want \"wal\"", path, journalMode)
	}
	var foreignKeys int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("database %s: foreign_keys is %d, want 1", path, foreignKeys)
	}

	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	ms, err := loadMigrations(sub)
	if err != nil {
		return err
	}
	return applyMigrations(ctx, db, ms)
}

// DB returns the connection pool for repositories.
func (s *Store) DB() *sql.DB { return s.db }

// SchemaVersion returns the highest applied migration version, 0 if none.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	return currentVersion(ctx, s.db)
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// dsn builds the modernc.org/sqlite connection string. Pragmas in the DSN
// are applied to every new connection of the pool, not just the first one.
func dsn(path string) string {
	return path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"
}
