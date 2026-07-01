package db

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/gorm"
)

// migratedDB opens a fresh SQLite database through the production Init path and
// applies the full migration chain, returning the connection. This is the real
// startup path (Init's DSN enables _foreign_keys=on).
func migratedDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	return gdb
}

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

// TestForeignKeyCascades verifies the ON DELETE CASCADE / SET NULL constraints
// added in migration 000032 actually fire — i.e. that Init enables enforcement
// (_foreign_keys=on) and the rebuilt junction/child tables carry the FKs. This
// is the behavior the removed hand-written cleanup now relies on.
func TestForeignKeyCascades(t *testing.T) {
	gdb := migratedDB(t)

	// Enforcement must be on, or every cascade below is a silent no-op.
	var fk int
	if err := gdb.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("read pragma: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys pragma = %d, want 1 (enforcement off — cascades won't fire)", fk)
	}

	exec := func(q string) {
		t.Helper()
		if err := gdb.Exec(q).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	count := func(q string) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q).Scan(&n).Error; err != nil {
			t.Fatalf("count %q: %v", q, err)
		}
		return n
	}

	// Seed one parent of each kind plus a child in every constrained table.
	exec("INSERT INTO devices(id) VALUES (1)")
	exec("INSERT INTO images(id, source, external_id) VALUES (10, 'immich', 'x')")
	exec("INSERT INTO albums(id, source, external_id, name) VALUES (100, 'immich', 'e', 'n')")
	exec("INSERT INTO url_sources(id, url) VALUES (500, 'http://u')")
	exec("INSERT INTO image_album_memberships(image_id, album_id) VALUES (10, 100)")
	exec("INSERT INTO device_album_mappings(device_id, album_id) VALUES (1, 100)")
	exec("INSERT INTO device_url_mappings(device_id, url_source_id) VALUES (1, 500)")
	exec("INSERT INTO generative_states(device_id, source) VALUES (1, 'fractal')")
	exec("INSERT INTO device_histories(device_id, image_id, served_at) VALUES (1, 10, '2026-01-01')")

	// Deleting the image cascades its membership and NULLs (not deletes) the
	// history reference.
	exec("DELETE FROM images WHERE id = 10")
	if got := count("SELECT COUNT(*) FROM image_album_memberships"); got != 0 {
		t.Errorf("membership survived image delete: got %d, want 0", got)
	}
	if got := count("SELECT COUNT(*) FROM device_histories WHERE image_id IS NULL"); got != 1 {
		t.Errorf("history image_id not set null: got %d rows null, want 1", got)
	}
	if got := count("SELECT COUNT(*) FROM device_histories"); got != 1 {
		t.Errorf("history row wrongly deleted on image delete: got %d, want 1", got)
	}

	// Deleting the album cascades the device_album_mapping.
	exec("DELETE FROM albums WHERE id = 100")
	if got := count("SELECT COUNT(*) FROM device_album_mappings"); got != 0 {
		t.Errorf("device_album_mapping survived album delete: got %d, want 0", got)
	}

	// Deleting the url source cascades the device_url_mapping.
	exec("DELETE FROM url_sources WHERE id = 500")
	if got := count("SELECT COUNT(*) FROM device_url_mappings"); got != 0 {
		t.Errorf("device_url_mapping survived url_source delete: got %d, want 0", got)
	}

	// Deleting the device cascades its remaining children.
	exec("DELETE FROM devices WHERE id = 1")
	if got := count("SELECT COUNT(*) FROM generative_states"); got != 0 {
		t.Errorf("generative_state survived device delete: got %d, want 0", got)
	}
	if got := count("SELECT COUNT(*) FROM device_histories"); got != 0 {
		t.Errorf("device_history survived device delete: got %d, want 0", got)
	}
}
