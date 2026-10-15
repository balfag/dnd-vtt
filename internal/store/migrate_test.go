package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(filepath.Join(t.TempDir(), "test.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustLoad(t *testing.T, files map[string]string) []migration {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	ms, err := loadMigrations(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func appliedAt(t *testing.T, db *sql.DB, version int) int64 {
	t.Helper()
	var at int64
	if err := db.QueryRow(`SELECT applied_at FROM schema_migrations WHERE version = ?`, version).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func countMigrations(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLoadMigrationsSorted(t *testing.T) {
	ms := mustLoad(t, map[string]string{
		"0002_b.sql": "SELECT 2;",
		"0001_a.sql": "SELECT 1;",
	})
	if len(ms) != 2 {
		t.Fatalf("len = %d, want 2", len(ms))
	}
	if ms[0].version != 1 || ms[0].name != "0001_a.sql" || ms[0].sql != "SELECT 1;" {
		t.Errorf("ms[0] = %+v", ms[0])
	}
	if ms[1].version != 2 || ms[1].name != "0002_b.sql" || ms[1].sql != "SELECT 2;" {
		t.Errorf("ms[1] = %+v", ms[1])
	}
}

func TestLoadMigrationsErrors(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{"version without padding", fstest.MapFS{"1_a.sql": {Data: []byte("x")}}},
		{"uppercase name", fstest.MapFS{"0001_A.sql": {Data: []byte("x")}}},
		{"wrong extension", fstest.MapFS{"0001_a.txt": {Data: []byte("x")}}},
		{"gap", fstest.MapFS{"0001_a.sql": {Data: []byte("x")}, "0003_c.sql": {Data: []byte("x")}}},
		{"duplicate", fstest.MapFS{"0001_a.sql": {Data: []byte("x")}, "0001_b.sql": {Data: []byte("x")}}},
		{"directory", fstest.MapFS{"0001_a.sql/x.sql": {Data: []byte("x")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadMigrations(tt.files); err == nil {
				t.Fatal("loadMigrations succeeded, want error")
			}
		})
	}
}

func TestLoadMigrationsEmpty(t *testing.T) {
	ms, err := loadMigrations(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 0 {
		t.Errorf("len = %d, want 0", len(ms))
	}
}

func TestLoadMigrationsEmbedded(t *testing.T) {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations(sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 || ms[0].version != 1 || ms[0].name != "0001_init.sql" {
		t.Fatalf("first embedded migration = %+v", ms)
	}
}

func TestApplyMigrations(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	ms := mustLoad(t, map[string]string{
		"0001_a.sql": "CREATE TABLE a (x INTEGER);",
		"0002_b.sql": "CREATE TABLE b (x INTEGER);",
	})

	if err := applyMigrations(ctx, db, ms); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "a") || !tableExists(t, db, "b") {
		t.Error("tables a and b must exist")
	}
	if v, err := currentVersion(ctx, db); err != nil || v != 2 {
		t.Errorf("currentVersion = %d, %v; want 2", v, err)
	}
	if n := countMigrations(t, db); n != 2 {
		t.Errorf("schema_migrations rows = %d, want 2", n)
	}
	if appliedAt(t, db, 1) <= 0 || appliedAt(t, db, 2) <= 0 {
		t.Error("applied_at must be positive")
	}

	// Idempotent.
	if err := applyMigrations(ctx, db, ms); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if n := countMigrations(t, db); n != 2 {
		t.Errorf("schema_migrations rows after reapply = %d, want 2", n)
	}
}

func TestApplyMigrationsIncremental(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	ms := mustLoad(t, map[string]string{
		"0001_a.sql": "CREATE TABLE a (x INTEGER);",
		"0002_b.sql": "CREATE TABLE b (x INTEGER);",
	})

	if err := applyMigrations(ctx, db, ms[:1]); err != nil {
		t.Fatal(err)
	}
	// Force a distinguishable timestamp so a re-application would show.
	if _, err := db.Exec(`UPDATE schema_migrations SET applied_at = 111 WHERE version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, db, ms); err != nil {
		t.Fatal(err)
	}
	if at := appliedAt(t, db, 1); at != 111 {
		t.Errorf("migration 1 applied_at = %d, want unchanged 111", at)
	}
	if !tableExists(t, db, "b") {
		t.Error("table b must exist")
	}
	if v, _ := currentVersion(ctx, db); v != 2 {
		t.Errorf("currentVersion = %d, want 2", v)
	}
}

func TestApplyMigrationsRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	ms := mustLoad(t, map[string]string{
		"0001_a.sql": "CREATE TABLE a (x INTEGER);",
		"0002_b.sql": "CREATE TABLE t2 (x INTEGER); CREATE TABLE broken (;",
	})

	err := applyMigrations(ctx, db, ms)
	if err == nil {
		t.Fatal("applyMigrations succeeded, want error")
	}
	if !strings.Contains(err.Error(), "0002_") {
		t.Errorf("error %q does not name the failing file", err)
	}
	if tableExists(t, db, "t2") {
		t.Error("table t2 must be rolled back")
	}
	if v, _ := currentVersion(ctx, db); v != 1 {
		t.Errorf("currentVersion = %d, want 1", v)
	}
}

func TestApplyMigrationsNewerDatabase(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	ms := mustLoad(t, map[string]string{
		"0001_a.sql": "CREATE TABLE a (x INTEGER);",
		"0002_b.sql": "CREATE TABLE b (x INTEGER);",
	})
	if err := applyMigrations(ctx, db, ms); err != nil {
		t.Fatal(err)
	}

	err := applyMigrations(ctx, db, ms[:1])
	if err == nil || !strings.Contains(err.Error(), "newer than this build supports") {
		t.Fatalf("err = %v, want \"newer than this build supports\"", err)
	}
}

func TestApplyMigrationsRenamed(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	orig := mustLoad(t, map[string]string{"0001_a.sql": "CREATE TABLE a (x INTEGER);"})
	if err := applyMigrations(ctx, db, orig); err != nil {
		t.Fatal(err)
	}

	renamed := mustLoad(t, map[string]string{"0001_renamed.sql": "CREATE TABLE a (x INTEGER);"})
	err := applyMigrations(ctx, db, renamed)
	if err == nil || !strings.Contains(err.Error(), "was applied as") {
		t.Fatalf("err = %v, want \"was applied as\"", err)
	}
}
