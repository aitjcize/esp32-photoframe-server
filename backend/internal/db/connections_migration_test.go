package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
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

// TestGoogleConnectionMigration seeds a v41 database with the single Google
// Photos token and a few imported photos, the way gorm wrote them, and checks
// that 000042 turns them into one Google connection whose state Go can decode,
// and that the way back restores the token row.
func TestGoogleConnectionMigration(t *testing.T) {
	gdb, m := migratedTo(t, 41)
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

	// gorm stores time.Time in this layout, with the local offset.
	expiry := time.Date(2026, 11, 15, 10, 30, 0, 0, time.FixedZone("TPE", 8*3600))
	exec("INSERT INTO google_auths(id, access_token, refresh_token, expiry) VALUES (1, 'acc', 'ref', ?)",
		expiry.Format("2006-01-02 15:04:05.999999999-07:00"))
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (20, 'google_photos', '', '/data/photos/g1.jpg')")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (21, 'google_photos', '', '/data/photos/g2.jpg')")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (22, 'gallery', '', '/data/photos/u.jpg')")

	if err := m.Migrate(42); err != nil {
		t.Fatalf("migrate to 42: %v", err)
	}

	if got := num("SELECT COUNT(*) FROM connections WHERE source = 'google_photos'"); got != 1 {
		t.Fatalf("google connections: got %d, want 1", got)
	}
	var conn model.Connection
	if err := gdb.Where("source = ?", "google_photos").First(&conn).Error; err != nil {
		t.Fatalf("load connection: %v", err)
	}
	if conn.Name != "Google Photos" || !conn.Enabled || conn.Config != "{}" {
		t.Errorf("connection row: %+v", conn)
	}
	st := conn.GoogleState()
	if st.AccessToken != "acc" || st.RefreshToken != "ref" {
		t.Errorf("token not carried over: %+v", st)
	}
	if !st.Expiry.Equal(expiry) {
		t.Errorf("expiry: got %v, want %v (state %s)", st.Expiry, expiry, conn.State)
	}
	if !st.Connected() {
		t.Errorf("migrated account should count as connected")
	}
	if got := num("SELECT COUNT(*) FROM images WHERE source = 'google_photos' AND connection_id = ?", conn.ID); got != 2 {
		t.Errorf("google images reparented: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE id = 22 AND connection_id IS NULL"); got != 1 {
		t.Errorf("gallery upload must keep a NULL connection")
	}
	if got := num("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'google_auths'"); got != 0 {
		t.Errorf("google_auths table should be gone")
	}

	// A second account fits beside the first.
	exec("INSERT INTO connections(source, name, enabled) VALUES ('google_photos', 'Work', 1)")
	second := num("SELECT MAX(id) FROM connections")
	exec("INSERT INTO images(source, external_id, file_path, connection_id) VALUES ('google_photos', '', '/data/photos/w.jpg', ?)", second)

	// Down: the first account's token is back in google_auths, every photo
	// survives detached, the connection rows are gone.
	if err := m.Migrate(41); err != nil {
		t.Fatalf("migrate down to 41: %v", err)
	}
	if got := str("SELECT refresh_token FROM google_auths WHERE id = 1"); got != "ref" {
		t.Errorf("down: refresh_token %q", got)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE source = 'google_photos' AND connection_id IS NULL"); got != 3 {
		t.Errorf("down: google images kept and detached: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM connections WHERE source = 'google_photos'"); got != 0 {
		t.Errorf("down: google connections left: %d", got)
	}
}

// Without a token and without imported photos no Google connection is made.
func TestGoogleConnectionMigrationOnFreshInstall(t *testing.T) {
	gdb, _ := migratedTo(t, 42)
	var n int
	if err := gdb.Raw("SELECT COUNT(*) FROM connections WHERE source = 'google_photos'").Scan(&n).Error; err != nil {
		t.Fatalf("connections table: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh install created %d google connections", n)
	}
}

// TestTopicConnectionsMigration seeds a v42 database with the flat Unsplash
// and Pexels settings, topic albums and photos, and checks that 000043 turns
// each source into one connection with its config, topics and photos, and
// that the down migration writes the first connection back to the keys.
func TestTopicConnectionsMigration(t *testing.T) {
	gdb, m := migratedTo(t, 42)
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

	// Unsplash: configured with a key and options, one topic with a photo.
	exec("INSERT INTO settings(key, value) VALUES ('unsplash_api_key', 'u-key')")
	exec("INSERT INTO settings(key, value) VALUES ('unsplash_randomize_results', 'true')")
	exec("INSERT INTO settings(key, value) VALUES ('unsplash_auto_sync_enabled', 'true')")
	exec("INSERT INTO settings(key, value) VALUES ('unsplash_auto_sync_interval', '180')")
	exec("INSERT INTO albums(id, source, external_id, name, kind, sync_enabled, cover_key, updated_at) VALUES (10, 'unsplash', 'cats', 'cats', 'real', 1, '', CURRENT_TIMESTAMP)")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (30, 'unsplash', 'u1', 'https://images.unsplash.com/u1')")
	// Pexels: the key was cleared but a topic and its photo remain.
	exec("INSERT INTO albums(id, source, external_id, name, kind, sync_enabled, cover_key, updated_at) VALUES (11, 'pexels', 'dogs', 'dogs', 'real', 1, '', CURRENT_TIMESTAMP)")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (31, 'pexels', '7', 'https://images.pexels.com/7')")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (32, 'gallery', '', '/data/photos/u.jpg')")

	if err := m.Migrate(43); err != nil {
		t.Fatalf("migrate to 43: %v", err)
	}

	var unsplash, pexels model.Connection
	if err := gdb.Where("source = ?", "unsplash").First(&unsplash).Error; err != nil {
		t.Fatalf("unsplash connection: %v", err)
	}
	if err := gdb.Where("source = ?", "pexels").First(&pexels).Error; err != nil {
		t.Fatalf("pexels connection: %v", err)
	}
	if unsplash.Name != "Unsplash" || !unsplash.Enabled {
		t.Errorf("unsplash row: %+v", unsplash)
	}
	cfg := unsplash.TopicConfig()
	if cfg.APIKey != "u-key" || !cfg.RandomizeResults || !cfg.AutoSyncEnabled || cfg.AutoSyncIntervalMinutes != 180 {
		t.Errorf("unsplash config not carried over: %+v (config %s)", cfg, unsplash.Config)
	}
	if pexels.Name != "Pexels" || pexels.Enabled {
		t.Errorf("pexels without a key must be disabled: %+v", pexels)
	}
	if pcfg := pexels.TopicConfig(); pcfg.APIKey != "" || pcfg.AutoSyncIntervalMinutes != 60 {
		t.Errorf("pexels config: %+v", pcfg)
	}
	if got := num("SELECT connection_id FROM albums WHERE id = 10"); got != int(unsplash.ID) {
		t.Errorf("unsplash topic reparented to %d, want %d", got, unsplash.ID)
	}
	if got := num("SELECT connection_id FROM images WHERE id = 30"); got != int(unsplash.ID) {
		t.Errorf("unsplash photo reparented to %d, want %d", got, unsplash.ID)
	}
	if got := num("SELECT connection_id FROM albums WHERE id = 11"); got != int(pexels.ID) {
		t.Errorf("pexels topic reparented to %d, want %d", got, pexels.ID)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE id = 32 AND connection_id IS NULL"); got != 1 {
		t.Errorf("gallery upload must keep a NULL connection")
	}
	if got := num("SELECT COUNT(*) FROM settings WHERE key LIKE 'unsplash_%' OR key LIKE 'pexels_%'"); got != 0 {
		t.Errorf("moved settings keys left behind: %d", got)
	}

	// A second Unsplash key may hold the same topic.
	exec("INSERT INTO connections(source, name, enabled, config) VALUES ('unsplash', 'Work', 1, '{\"api_key\":\"w-key\"}')")
	second := num("SELECT MAX(id) FROM connections")
	exec("INSERT INTO albums(source, external_id, name, kind, sync_enabled, cover_key, updated_at, connection_id) VALUES ('unsplash', 'cats', 'cats', 'real', 1, '', CURRENT_TIMESTAMP, ?)", second)
	exec("INSERT INTO images(source, external_id, file_path, connection_id) VALUES ('unsplash', 'u1', 'https://images.unsplash.com/u1', ?)", second)

	// Down: the first key is back in settings, its rows are detached, the
	// second connection's rows (which would collide) are gone.
	if err := m.Migrate(42); err != nil {
		t.Fatalf("migrate down to 42: %v", err)
	}
	if got := str("SELECT value FROM settings WHERE key = 'unsplash_api_key'"); got != "u-key" {
		t.Errorf("down: unsplash_api_key %q", got)
	}
	if got := str("SELECT value FROM settings WHERE key = 'unsplash_auto_sync_interval'"); got != "180" {
		t.Errorf("down: unsplash_auto_sync_interval %q", got)
	}
	if got := str("SELECT value FROM settings WHERE key = 'unsplash_randomize_results'"); got != "true" {
		t.Errorf("down: unsplash_randomize_results %q", got)
	}
	if got := num("SELECT COUNT(*) FROM albums WHERE source = 'unsplash'"); got != 1 {
		t.Errorf("down: unsplash albums left: %d, want the first connection's only", got)
	}
	if got := num("SELECT COUNT(*) FROM images WHERE source = 'unsplash' AND connection_id IS NULL"); got != 1 {
		t.Errorf("down: unsplash images kept and detached: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM albums WHERE id = 11 AND connection_id IS NULL"); got != 1 {
		t.Errorf("down: pexels topic should survive detached")
	}
	if got := num("SELECT COUNT(*) FROM connections WHERE source IN ('unsplash', 'pexels')"); got != 0 {
		t.Errorf("down: topic connections left: %d", got)
	}
}

// Without a key and without topics or photos no topic connection is made.
func TestTopicConnectionsMigrationOnFreshInstall(t *testing.T) {
	gdb, _ := migratedTo(t, 43)
	var n int
	if err := gdb.Raw("SELECT COUNT(*) FROM connections WHERE source IN ('unsplash', 'pexels')").Scan(&n).Error; err != nil {
		t.Fatalf("connections table: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh install created %d topic connections", n)
	}
}

// TestCalendarConnectionMigration seeds a v43 database with the single Google
// Calendar token and frames that show a calendar, and checks that 000044
// turns the token into one google_calendar connection, points every
// calendar-showing frame at it, and leaves the others unbound.
func TestCalendarConnectionMigration(t *testing.T) {
	gdb, m := migratedTo(t, 43)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := gdb.Exec(q, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	num := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q, args...).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}
	str := func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := gdb.Raw(q, args...).Scan(&s).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}

	expiry := time.Date(2026, 11, 15, 10, 30, 0, 0, time.FixedZone("TPE", 8*3600))
	exec("INSERT INTO google_calendar_auths(id, access_token, refresh_token, expiry) VALUES (1, 'acc', 'ref', ?)",
		expiry.Format("2006-01-02 15:04:05.999999999-07:00"))
	exec("INSERT INTO devices(id, name, host, show_calendar, calendar_id) VALUES (1, 'kitchen', '10.0.0.1', 1, 'family@group.calendar.google.com')")
	exec("INSERT INTO devices(id, name, host, show_calendar, calendar_id) VALUES (2, 'study', '10.0.0.2', 0, '')")

	if err := m.Migrate(44); err != nil {
		t.Fatalf("migrate to 44: %v", err)
	}

	if got := num("SELECT COUNT(*) FROM connections WHERE source = 'google_calendar'"); got != 1 {
		t.Fatalf("calendar connections: got %d, want 1", got)
	}
	var conn model.Connection
	if err := gdb.Where("source = ?", "google_calendar").First(&conn).Error; err != nil {
		t.Fatalf("load connection: %v", err)
	}
	if conn.Name != "Google Calendar" || !conn.Enabled || conn.Config != "{}" {
		t.Errorf("connection row: %+v", conn)
	}
	st := conn.GoogleState()
	if st.AccessToken != "acc" || st.RefreshToken != "ref" || !st.Expiry.Equal(expiry) || !st.Connected() {
		t.Errorf("token not carried over: %+v (state %s)", st, conn.State)
	}
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 1 AND calendar_connection_id = ?", conn.ID); got != 1 {
		t.Errorf("the frame showing a calendar must point at the migrated account")
	}
	if got := str("SELECT calendar_id FROM devices WHERE id = 1"); got != "family@group.calendar.google.com" {
		t.Errorf("calendar_id must survive: %q", got)
	}
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 2 AND calendar_connection_id IS NULL"); got != 1 {
		t.Errorf("a frame without a calendar stays unbound")
	}
	if got := num("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'google_calendar_auths'"); got != 0 {
		t.Errorf("google_calendar_auths table should be gone")
	}

	// A second account fits beside the first; the frame on it is unbound
	// when it goes (ON DELETE SET NULL), not deleted.
	exec("INSERT INTO connections(source, name, enabled, state) VALUES ('google_calendar', 'Work', 1, '{\"refresh_token\":\"r2\"}')")
	second := num("SELECT MAX(id) FROM connections")
	exec("UPDATE devices SET calendar_connection_id = ? WHERE id = 2", second)
	exec("DELETE FROM connections WHERE id = ?", second)
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 2 AND calendar_connection_id IS NULL"); got != 1 {
		t.Errorf("deleting an account must unbind its frames, not remove them")
	}
	if got := num("SELECT COUNT(*) FROM devices"); got != 2 {
		t.Errorf("devices: %d", got)
	}

	if err := m.Migrate(43); err != nil {
		t.Fatalf("migrate down to 43: %v", err)
	}
	if got := str("SELECT refresh_token FROM google_calendar_auths WHERE id = 1"); got != "ref" {
		t.Errorf("down: refresh token %q", got)
	}
	if got := num("SELECT COUNT(*) FROM connections WHERE source = 'google_calendar'"); got != 0 {
		t.Errorf("down: calendar connections left: %d", got)
	}
	if got := num("SELECT COUNT(*) FROM devices WHERE show_calendar = 1 AND calendar_id = 'family@group.calendar.google.com'"); got != 1 {
		t.Errorf("down: the frame's calendar settings must survive")
	}
}

// Without a calendar token no calendar connection is made and frames stay
// unbound.
func TestCalendarConnectionMigrationOnFreshInstall(t *testing.T) {
	gdb, m := migratedTo(t, 43)
	if err := gdb.Exec("INSERT INTO devices(id, name, host, show_calendar) VALUES (1, 'kitchen', '10.0.0.1', 1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(44); err != nil {
		t.Fatalf("migrate to 44: %v", err)
	}
	var n int
	gdb.Raw("SELECT COUNT(*) FROM connections WHERE source = 'google_calendar'").Scan(&n)
	if n != 0 {
		t.Errorf("fresh install must not get a calendar connection, got %d", n)
	}
	gdb.Raw("SELECT COUNT(*) FROM devices WHERE calendar_connection_id IS NULL").Scan(&n)
	if n != 1 {
		t.Errorf("frame must stay unbound")
	}
}

// 000045 binds a frame to its source's only connection and leaves a frame
// whose source has several unbound (it keeps drawing from all of them);
// frames on built-in sources are untouched, and a deleted connection unbinds
// its frames.
func TestImageConnectionMigration(t *testing.T) {
	gdb, m := migratedTo(t, 44)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := gdb.Exec(q, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	num := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q, args...).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}

	exec("INSERT INTO connections(id, source, name, enabled) VALUES (1, 'immich', 'Home', 1)")
	exec("INSERT INTO connections(id, source, name, enabled) VALUES (2, 'immich', 'Parents', 1)")
	exec("INSERT INTO connections(id, source, name, enabled) VALUES (3, 'synology_photos', 'NAS', 1)")
	exec("INSERT INTO devices(id, name, host, source) VALUES (1, 'kitchen', '10.0.0.1', 'immich')")
	exec("INSERT INTO devices(id, name, host, source) VALUES (2, 'study', '10.0.0.2', 'synology_photos')")
	exec("INSERT INTO devices(id, name, host, source) VALUES (3, 'hall', '10.0.0.3', 'gallery')")
	exec("INSERT INTO devices(id, name, host, source) VALUES (4, 'new', '10.0.0.4', '')")

	if err := m.Migrate(45); err != nil {
		t.Fatalf("migrate to 45: %v", err)
	}
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM devices WHERE id = 1 AND image_connection_id IS NULL": 1, // two Immich servers: any
		"SELECT COUNT(*) FROM devices WHERE id = 2 AND image_connection_id = 3":     1, // the only NAS
		"SELECT COUNT(*) FROM devices WHERE id = 3 AND image_connection_id IS NULL": 1,
		"SELECT COUNT(*) FROM devices WHERE id = 4 AND image_connection_id IS NULL": 1,
	} {
		if got := num(q); got != want {
			t.Errorf("%s = %d, want %d", q, got, want)
		}
	}

	exec("UPDATE devices SET image_connection_id = 2 WHERE id = 1")
	exec("DELETE FROM connections WHERE id = 2")
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 1 AND image_connection_id IS NULL"); got != 1 {
		t.Errorf("deleting a connection must unbind its frames, not remove them")
	}
	if got := num("SELECT COUNT(*) FROM devices"); got != 4 {
		t.Errorf("devices: %d", got)
	}

	if err := m.Migrate(44); err != nil {
		t.Fatalf("migrate down to 44: %v", err)
	}
	if got := num("SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name = 'image_connection_id'"); got != 0 {
		t.Errorf("down: column must be gone")
	}
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 2 AND source = 'synology_photos'"); got != 1 {
		t.Errorf("down: the frame's source must survive")
	}
}

// A rollback must leave a schema the application can still write to, and a
// later upgrade must apply again: 000041's down rebuilds albums and images
// without their connection_id (a dangling REFERENCES would fail every insert
// once connections is gone), keeping memberships, frame bindings and the
// serving history; 000044's and 000045's downs do the same for devices.
func TestConnectionRollbacksKeepTheSchemaUsable(t *testing.T) {
	gdb, m := migratedTo(t, 45)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := gdb.Exec(q, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	num := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q, args...).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}

	exec("INSERT INTO connections(id, source, name, enabled) VALUES (1, 'immich', 'Immich', 1)")
	exec("INSERT INTO connections(id, source, name, enabled, state) VALUES (2, 'google_calendar', 'Cal', 1, '{\"refresh_token\":\"r\"}')")
	exec("INSERT INTO devices(id, name, host, source, image_connection_id, show_calendar, calendar_connection_id) VALUES (1, 'kitchen', '10.0.0.1', 'immich', 1, 1, 2)")
	exec("INSERT INTO albums(id, source, external_id, name, sync_enabled, connection_id) VALUES (1, 'immich', 'a', 'Trips', 1, 1)")
	exec("INSERT INTO albums(id, source, external_id, name, sync_enabled) VALUES (2, 'gallery', 'gallery-x', 'Family', 1)")
	exec("INSERT INTO images(id, source, external_id, file_path, connection_id) VALUES (1, 'immich', 'i1', '', 1)")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (2, 'gallery', '', '/data/photos/gallery/u.jpg')")
	exec("INSERT INTO image_album_memberships(image_id, album_id) VALUES (1, 1), (2, 2)")
	exec("INSERT INTO device_album_mappings(device_id, album_id) VALUES (1, 1), (1, 2)")
	exec("INSERT INTO device_histories(device_id, image_id, served_at) VALUES (1, 2, CURRENT_TIMESTAMP)")
	exec("INSERT INTO api_keys(id, user_id, name, created_at, device_id) VALUES (1, 1, 'frame', CURRENT_TIMESTAMP, 1)")
	// A revoked key (its row is gone) must stay revoked: its id is never
	// issued again, even though the table is rebuilt on the way down.
	exec("INSERT INTO api_keys(id, user_id, name, created_at, device_id) VALUES (2, 1, 'revoked', CURRENT_TIMESTAMP, 1)")
	exec("DELETE FROM api_keys WHERE id = 2")
	exec("INSERT INTO devices(id, name, host) VALUES (2, 'gone', '10.0.0.9')")
	exec("DELETE FROM devices WHERE id = 2")
	exec("INSERT INTO images(id, source, external_id, file_path) VALUES (3, 'gallery', '', '/data/photos/gallery/gone.jpg')")
	exec("DELETE FROM images WHERE id = 3")
	exec("INSERT INTO url_sources(id, url) VALUES (1, 'http://x/y.jpg')")
	exec("INSERT INTO device_url_mappings(device_id, url_source_id) VALUES (1, 1)")
	exec("INSERT INTO generative_states(device_id, source, state) VALUES (1, 'fractal', x'00')")

	// Down past the image connection column: the calendar binding and the
	// children of devices survive the rebuild.
	if err := m.Migrate(44); err != nil {
		t.Fatalf("migrate down to 44: %v", err)
	}
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM devices WHERE id = 1 AND calendar_connection_id = 2":             1,
		"SELECT COUNT(*) FROM api_keys WHERE id = 1 AND device_id = 1":                         1,
		"SELECT COUNT(*) FROM device_album_mappings WHERE device_id = 1":                       2,
		"SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name = 'image_connection_id'": 0,
	} {
		if got := num(q); got != want {
			t.Errorf("after down to 44: %s = %d, want %d", q, got, want)
		}
	}

	// Down past the calendar column: devices and all its children survive.
	if err := m.Migrate(43); err != nil {
		t.Fatalf("migrate down to 43: %v", err)
	}
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM devices WHERE id = 1 AND show_calendar = 1":                         1,
		"SELECT COUNT(*) FROM api_keys WHERE id = 1 AND device_id = 1":                            1,
		"SELECT COUNT(*) FROM device_album_mappings WHERE device_id = 1":                          2,
		"SELECT COUNT(*) FROM device_url_mappings WHERE device_id = 1":                            1,
		"SELECT COUNT(*) FROM generative_states WHERE device_id = 1":                              1,
		"SELECT COUNT(*) FROM device_histories WHERE device_id = 1 AND image_id = 2":              1,
		"SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name = 'calendar_connection_id'": 0,
	} {
		if got := num(q); got != want {
			t.Errorf("after down to 43: %s = %d, want %d", q, got, want)
		}
	}
	exec("INSERT INTO api_keys(user_id, name, created_at, device_id) VALUES (1, 'new', CURRENT_TIMESTAMP, 1)")
	if got := num("SELECT MAX(id) FROM api_keys"); got != 3 {
		t.Errorf("after down to 43: a new api key got id %d; the revoked id 2 must not be reused", got)
	}
	exec("INSERT INTO devices(name, host) VALUES ('study', '10.0.0.2')")
	if got := num("SELECT id FROM devices WHERE host = '10.0.0.2'"); got != 3 {
		t.Errorf("after down to 43: a new device got id %d, want 3", got)
	}
	exec("DELETE FROM devices WHERE host = '10.0.0.2'")

	// Down past the connections table: inserts still work and the children
	// of albums and images are intact.
	if err := m.Migrate(40); err != nil {
		t.Fatalf("migrate down to 40: %v", err)
	}
	exec("INSERT INTO images(source, external_id, file_path) VALUES ('gallery', '', '/data/photos/gallery/v.jpg')")
	if got := num("SELECT MAX(id) FROM images"); got != 4 {
		t.Errorf("after down to 40: a new image got id %d; the deleted id 3 must not be reused", got)
	}
	exec("INSERT INTO albums(source, external_id, name) VALUES ('immich', 'b', 'More')")
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM images":                                                      3,
		"SELECT COUNT(*) FROM albums":                                                      3,
		"SELECT COUNT(*) FROM image_album_memberships":                                     2,
		"SELECT COUNT(*) FROM device_album_mappings WHERE device_id = 1":                   2,
		"SELECT COUNT(*) FROM device_histories WHERE device_id = 1 AND image_id = 2":       1,
		"SELECT COUNT(*) FROM pragma_table_info('images') WHERE name = 'connection_id'":    0,
		"SELECT COUNT(*) FROM pragma_table_info('albums') WHERE name = 'connection_id'":    0,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'connections'": 0,
	} {
		if got := num(q); got != want {
			t.Errorf("after down to 40: %s = %d, want %d", q, got, want)
		}
	}
	// Cascades still fire on the rebuilt tables.
	exec("DELETE FROM images WHERE id = 2")
	if got := num("SELECT COUNT(*) FROM image_album_memberships WHERE image_id = 2"); got != 0 {
		t.Errorf("membership must cascade with its image")
	}
	if got := num("SELECT COUNT(*) FROM device_histories WHERE image_id IS NULL"); got != 1 {
		t.Errorf("history must keep the row and null the image")
	}

	// And back up to the top.
	if err := m.Migrate(45); err != nil {
		t.Fatalf("migrate back up to 45: %v", err)
	}
	if got := num("SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name = 'calendar_connection_id'"); got != 1 {
		t.Errorf("calendar column must be back")
	}
	if got := num("SELECT COUNT(*) FROM devices WHERE id = 1 AND image_connection_id = 1"); got != 1 {
		t.Errorf("the frame on the only Immich connection is bound to it again")
	}
	if got := num("SELECT COUNT(*) FROM connections WHERE source = 'immich'"); got != 1 {
		t.Errorf("the immich albums bring a connection back, got %d", got)
	}
}

// Deleting a connection on the real schema clears every frame's remembered
// selection of its albums (device_album_mappings cascade with the albums,
// which cascade with the connection), and unbinds the calendar and image
// bindings that point at it; the frames themselves stay.
func TestDeletingAConnectionClearsRememberedSelections(t *testing.T) {
	gdb, _ := migratedTo(t, 45)
	exec := func(q string, args ...any) {
		t.Helper()
		if err := gdb.Exec(q, args...).Error; err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	num := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := gdb.Raw(q, args...).Scan(&n).Error; err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return n
	}
	exec("INSERT INTO connections(id, source, name, enabled) VALUES (1, 'immich', 'Home', 1)")
	exec("INSERT INTO connections(id, source, name, enabled) VALUES (2, 'immich', 'Parents', 1)")
	exec("INSERT INTO albums(id, source, external_id, name, sync_enabled, connection_id) VALUES (1, 'immich', 'a', 'A', 1, 1)")
	exec("INSERT INTO albums(id, source, external_id, name, sync_enabled, connection_id) VALUES (2, 'immich', 'p', 'P', 1, 2)")
	exec("INSERT INTO devices(id, name, host, source, image_connection_id) VALUES (1, 'study', '10.0.0.1', 'immich', 2)")
	exec("INSERT INTO device_album_mappings(device_id, album_id) VALUES (1, 1), (1, 2)")

	exec("DELETE FROM connections WHERE id = 1")
	for q, want := range map[string]int{
		"SELECT COUNT(*) FROM devices WHERE id = 1 AND image_connection_id = 2": 1,
		"SELECT COUNT(*) FROM albums WHERE connection_id = 1":                   0,
		"SELECT COUNT(*) FROM device_album_mappings WHERE album_id = 1":         0,
		"SELECT COUNT(*) FROM device_album_mappings WHERE album_id = 2":         1,
	} {
		if got := num(q); got != want {
			t.Errorf("%s = %d, want %d", q, got, want)
		}
	}
}
