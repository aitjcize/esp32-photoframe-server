package db

import (
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"gorm.io/gorm"
)

// migratedTo opens a fresh database and applies the chain up to version v.
func migratedTo(t *testing.T, v uint) (*gorm.DB, *migrate.Migrate) {
	t.Helper()
	gdb, err := Init(filepath.Join(t.TempDir(), "m.db"))
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
	if err := m.Migrate(v); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to %d: %v", v, err)
	}
	return gdb, m
}

// TestConnectionsMigrationMovesGlobalSettings seeds a v40 database the way a
// real install looks (flat immich_*/synology_* settings, albums and images
// keyed by source) and checks that 000041 turns each configured provider into
// one connection, reparents its rows, seeds the legacy album, and removes the
// moved keys.
func TestConnectionsMigrationMovesGlobalSettings(t *testing.T) {
	gdb, m := migratedTo(t, 40)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := gdb.Exec(q, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	str := func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := gdb.Raw(q, args...).Scan(&s).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	num := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q, args...).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}

	for k, v := range map[string]string{
		"immich_url":                          "http://immich.local",
		"immich_api_key":                      "immich-key",
		"immich_memory_mode":                  "latest",
		"immich_auto_sync_enabled":            "true",
		"immich_auto_sync_interval_minutes":   "30",
		"synology_url":                        "https://nas.local:5001",
		"synology_account":                    "frame",
		"synology_password":                   "pw",
		"synology_skip_cert":                  "true",
		"synology_album_id":                   "77",
		"synology_sid":                        "sid-1",
		"synology_did":                        "did-1",
		"synology_token":                      "tok-1",
		"synology_auto_sync_interval_minutes": "",
		"google_client_id":                    "keep-me",
	} {
		exec("INSERT INTO settings(key, value) VALUES (?, ?)", k, v)
	}
	// Immich already uses the album picker: two albums, one of them virtual.
	exec("INSERT INTO albums(id, source, external_id, name, kind, sync_enabled) VALUES (1, 'immich', 'uuid-a', 'A', 'album', 1)")
	exec("INSERT INTO albums(id, source, external_id, name, kind, sync_enabled) VALUES (2, 'immich', '__all__', 'All Photos', 'virtual', 1)")
	exec("INSERT INTO images(id, source, external_id) VALUES (10, 'immich', 'asset-1')")
	exec("INSERT INTO images(id, source, external_id) VALUES (11, 'synology_photos', '500')")
	// A topic album and an upload must be left alone.
	exec("INSERT INTO albums(id, source, external_id, name, kind, sync_enabled) VALUES (3, 'unsplash', 'cats', 'cats', 'album', 1)")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (12, 'gallery', '', '/data/photos/x.jpg')")

	if err := m.Migrate(41); err != nil {
		t.Fatalf("migrate to 41: %v", err)
	}

	if got := num("SELECT COUNT(*) FROM connections"); got != 2 {
		t.Fatalf("connections: got %d, want 2", got)
	}
	immichID := num("SELECT id FROM connections WHERE source = 'immich'")
	synoID := num("SELECT id FROM connections WHERE source = 'synology_photos'")

	// Immich config carried over, with the right JSON types.
	if got := str("SELECT json_extract(config, '$.url') FROM connections WHERE id = ?", immichID); got != "http://immich.local" {
		t.Errorf("immich url: %q", got)
	}
	if got := str("SELECT json_extract(config, '$.api_key') FROM connections WHERE id = ?", immichID); got != "immich-key" {
		t.Errorf("immich api_key: %q", got)
	}
	if got := str("SELECT json_extract(config, '$.memory_mode') FROM connections WHERE id = ?", immichID); got != "latest" {
		t.Errorf("immich memory_mode: %q", got)
	}
	if got := str("SELECT json_type(config, '$.auto_sync_enabled') FROM connections WHERE id = ?", immichID); got != "true" {
		t.Errorf("immich auto_sync_enabled json type: %q, want true", got)
	}
	if got := num("SELECT json_extract(config, '$.auto_sync_interval_minutes') FROM connections WHERE id = ?", immichID); got != 30 {
		t.Errorf("immich interval: %d", got)
	}
	if got := num("SELECT enabled FROM connections WHERE id = ?", immichID); got != 1 {
		t.Errorf("immich enabled: %d", got)
	}

	// Synology: credentials, skip_cert as a bool, empty interval -> 60, session
	// in state.
	if got := str("SELECT json_extract(config, '$.account') FROM connections WHERE id = ?", synoID); got != "frame" {
		t.Errorf("synology account: %q", got)
	}
	if got := str("SELECT json_type(config, '$.skip_cert') FROM connections WHERE id = ?", synoID); got != "true" {
		t.Errorf("synology skip_cert json type: %q", got)
	}
	if got := num("SELECT json_extract(config, '$.auto_sync_interval_minutes') FROM connections WHERE id = ?", synoID); got != 60 {
		t.Errorf("synology interval default: %d", got)
	}
	if got := str("SELECT json_extract(state, '$.sid') FROM connections WHERE id = ?", synoID); got != "sid-1" {
		t.Errorf("synology sid: %q", got)
	}

	// Rows reparented; other sources untouched.
	if got := num("SELECT COUNT(*) FROM albums WHERE source = 'immich' AND connection_id = ?", immichID); got != 2 {
		t.Errorf("immich albums reparented: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE connection_id = ?", immichID); got != 1 {
		t.Errorf("immich images reparented: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE id = 11 AND connection_id = ?", synoID); got != 1 {
		t.Errorf("synology image reparented")
	}
	if got := num("SELECT COUNT(*) FROM albums WHERE id = 3 AND connection_id IS NULL"); got != 1 {
		t.Errorf("topic album must keep a NULL connection")
	}
	if got := num("SELECT COUNT(*) FROM images WHERE id = 12 AND connection_id IS NULL"); got != 1 {
		t.Errorf("gallery upload must keep a NULL connection")
	}

	// Legacy single-album seed: Synology had no album rows, so synology_album_id
	// becomes one; Immich had rows, so nothing is added there.
	if got := num("SELECT COUNT(*) FROM albums WHERE source = 'synology_photos' AND external_id = '77' AND sync_enabled = 1 AND connection_id = ?", synoID); got != 1 {
		t.Errorf("synology legacy album not seeded")
	}
	if got := num("SELECT COUNT(*) FROM albums WHERE source = 'immich'"); got != 2 {
		t.Errorf("immich albums: got %d, want the 2 existing ones only", got)
	}

	// Moved keys are gone, unrelated keys stay.
	if got := num("SELECT COUNT(*) FROM settings WHERE key LIKE 'immich_%' OR key LIKE 'synology_%'"); got != 0 {
		t.Errorf("%d provider settings keys survived", got)
	}
	if got := str("SELECT value FROM settings WHERE key = 'google_client_id'"); got != "keep-me" {
		t.Errorf("google_client_id lost: %q", got)
	}

	// The point of it all: the same external id may now exist once per
	// connection, while a second row within one connection is still refused.
	exec("INSERT INTO connections(source, name, enabled) VALUES ('immich', 'Immich 2', 1)")
	second := num("SELECT MAX(id) FROM connections")
	exec("INSERT INTO albums(source, external_id, name, kind, connection_id) VALUES ('immich', '__all__', 'All Photos', 'virtual', ?)", second)
	exec("INSERT INTO images(source, external_id, connection_id) VALUES ('immich', 'asset-1', ?)", second)
	if err := gdb.Exec("INSERT INTO albums(source, external_id, name, kind, connection_id) VALUES ('immich', '__all__', 'dup', 'virtual', ?)", second).Error; err == nil {
		t.Errorf("duplicate album within one connection was accepted")
	}
	if err := gdb.Exec("INSERT INTO images(source, external_id, connection_id) VALUES ('immich', 'asset-1', ?)", second).Error; err == nil {
		t.Errorf("duplicate image within one connection was accepted")
	}

	// Deleting a connection takes its albums and images with it.
	exec("DELETE FROM connections WHERE id = ?", second)
	if got := num("SELECT COUNT(*) FROM albums WHERE connection_id = ?", second); got != 0 {
		t.Errorf("albums survived connection delete")
	}
	if got := num("SELECT COUNT(*) FROM images WHERE connection_id = ?", second); got != 0 {
		t.Errorf("images survived connection delete")
	}

	// And the way back restores the first connection's settings.
	if err := m.Migrate(40); err != nil {
		t.Fatalf("migrate down to 40: %v", err)
	}
	if got := str("SELECT value FROM settings WHERE key = 'immich_url'"); got != "http://immich.local" {
		t.Errorf("down: immich_url %q", got)
	}
	if got := str("SELECT value FROM settings WHERE key = 'synology_sid'"); got != "sid-1" {
		t.Errorf("down: synology_sid %q", got)
	}
	if got := num("SELECT COUNT(*) FROM albums WHERE source = 'immich'"); got != 2 {
		t.Errorf("down: immich albums %d", got)
	}
}

// TestConnectionsMigrationOnFreshInstall: no settings, no rows -> no
// connections, and the schema is still sound.
func TestConnectionsMigrationOnFreshInstall(t *testing.T) {
	gdb, m := migratedTo(t, 41)
	var n int
	if err := gdb.Raw("SELECT COUNT(*) FROM connections").Scan(&n).Error; err != nil {
		t.Fatalf("connections table: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh install created %d connections", n)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("remaining migrations: %v", err)
	}
}
