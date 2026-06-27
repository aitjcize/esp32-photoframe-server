package db

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// migrationsURL resolves the real db/migrations directory relative to this
// test file, independent of the test's working directory.
func migrationsURL(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller path")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
	return "file://" + dir
}

// TestMigrationsApplyCleanly applies the full numbered migration chain against a
// fresh SQLite database. This is the production startup path and had no coverage;
// a broken or out-of-order migration would otherwise only surface on a real boot.
func TestMigrationsApplyCleanly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	gdb, err := Init(dbPath)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	driver, err := sqlite3.WithInstance(sqlDB, &sqlite3.Config{})
	if err != nil {
		t.Fatalf("sqlite3 driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance(migrationsURL(t), "sqlite3", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up failed: %v", err)
	}

	// Sanity: a table created by a late migration exists...
	var n int
	if err := gdb.Raw("SELECT COUNT(*) FROM albums").Scan(&n).Error; err != nil {
		t.Fatalf("albums table missing after migrate: %v", err)
	}
	// ...and the column dropped by 000027 is gone.
	rows, err := gdb.Raw("PRAGMA table_info(albums)").Rows()
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == "asset_count" {
			t.Fatal("albums.asset_count should have been dropped by migration 000027")
		}
	}
}
