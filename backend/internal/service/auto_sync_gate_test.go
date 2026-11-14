package service

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

var gateTestDBCounter atomic.Int64

// setupGateDB returns an isolated in-memory DB with just the tables the
// auto-sync gate reads.
func setupGateDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := gateTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:gate_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Connection{}, &model.Album{}))
	return db
}

// mkConnection stores a connection row with the given config and returns it.
func mkConnection(t *testing.T, db *gorm.DB, source, name string, cfg any) model.Connection {
	t.Helper()
	c := model.Connection{Source: source, Name: name, Enabled: true}
	require.NoError(t, c.EncodeConfig(cfg))
	require.NoError(t, NewConnectionStore(db).Create(&c))
	return c
}

// mkConnAlbum creates an album row owned by a connection.
func mkConnAlbum(t *testing.T, db *gorm.DB, c model.Connection, ext, kind string, enabled bool) {
	t.Helper()
	id := c.ID
	require.NoError(t, db.Create(&model.Album{
		Source: c.Source, ConnectionID: &id, ExternalID: ext, Kind: kind, Name: ext, SyncEnabled: enabled,
	}).Error)
}

func immichGateService(t *testing.T, db *gorm.DB) (*ImmichService, model.Connection) {
	t.Helper()
	c := mkConnection(t, db, model.SourceImmich, "Immich", model.ImmichConfig{
		URL: "http://immich.local", APIKey: "key",
	})
	return NewImmichService(db, NewConnectionStore(db), c.ID), c
}

func synologyGateService(t *testing.T, db *gorm.DB) (*SynologyService, model.Connection) {
	t.Helper()
	c := mkConnection(t, db, model.SourceSynologyPhotos, "NAS", model.SynologyConfig{
		URL: "http://nas.local", Account: "user", Password: "pass",
	})
	return NewSynologyService(db, NewConnectionStore(db), c.ID), c
}

// Issue #54: picking sources through the multi-album picker writes album rows;
// an enabled row, real or virtual, is what makes the connection configured.
func TestImmichAutoSyncConfiguredFromAlbumRows(t *testing.T) {
	db := setupGateDB(t)
	svc, c := immichGateService(t, db)

	mkConnAlbum(t, db, c, model.ImmichVirtualMemories, model.AlbumKindVirtual, true)

	assert.True(t, svc.isAutoSyncConfigured(),
		"an enabled virtual album must count as configured")
}

func TestImmichAutoSyncConfiguredFromRealAlbumRow(t *testing.T) {
	db := setupGateDB(t)
	svc, c := immichGateService(t, db)

	mkConnAlbum(t, db, c, "album-uuid", model.AlbumKindReal, true)

	assert.True(t, svc.isAutoSyncConfigured())
}

// Everything deselected through the picker means "nothing to sync".
func TestImmichAutoSyncNotConfiguredWhenAllAlbumsDeselected(t *testing.T) {
	db := setupGateDB(t)
	svc, c := immichGateService(t, db)

	mkConnAlbum(t, db, c, "album-uuid", model.AlbumKindReal, false)
	mkConnAlbum(t, db, c, model.ImmichVirtualAll, model.AlbumKindVirtual, false)

	assert.False(t, svc.isAutoSyncConfigured())
}

// No rows at all: the picker has never been used, nothing to sync. (The legacy
// single-album settings became a row in the connections migration, so there is
// no settings fallback any more.)
func TestImmichAutoSyncNotConfiguredWithoutAlbumRows(t *testing.T) {
	db := setupGateDB(t)
	svc, _ := immichGateService(t, db)

	assert.False(t, svc.isAutoSyncConfigured())
}

func TestImmichAutoSyncNotConfiguredWithoutCredentials(t *testing.T) {
	db := setupGateDB(t)
	svc, c := immichGateService(t, db)
	mkConnAlbum(t, db, c, "album-uuid", model.AlbumKindReal, true)

	require.NoError(t, c.EncodeConfig(model.ImmichConfig{URL: "http://immich.local"}))
	require.NoError(t, NewConnectionStore(db).Update(&c))

	assert.False(t, svc.isAutoSyncConfigured())
}

// A disabled connection never syncs, whatever its albums say.
func TestImmichAutoSyncNotConfiguredWhenDisabled(t *testing.T) {
	db := setupGateDB(t)
	svc, c := immichGateService(t, db)
	mkConnAlbum(t, db, c, "album-uuid", model.AlbumKindReal, true)

	c.Enabled = false
	require.NoError(t, NewConnectionStore(db).Update(&c))

	assert.False(t, svc.isAutoSyncConfigured())
}

// The gate must read its own connection's albums only: not another source's,
// and not another connection's of the same source.
func TestImmichAutoSyncIgnoresOtherConnectionsAlbums(t *testing.T) {
	db := setupGateDB(t)
	svc, _ := immichGateService(t, db)

	other := mkConnection(t, db, model.SourceImmich, "Immich 2", model.ImmichConfig{
		URL: "http://other.local", APIKey: "key",
	})
	mkConnAlbum(t, db, other, "album-uuid", model.AlbumKindReal, true)
	nas := mkConnection(t, db, model.SourceSynologyPhotos, "NAS", model.SynologyConfig{})
	mkConnAlbum(t, db, nas, "42", model.AlbumKindReal, true)

	assert.False(t, svc.isAutoSyncConfigured())
}

func TestSynologyAutoSyncConfiguredFromAlbumRows(t *testing.T) {
	db := setupGateDB(t)
	svc, c := synologyGateService(t, db)

	mkConnAlbum(t, db, c, "42", model.AlbumKindReal, true)

	assert.True(t, svc.isAutoSyncConfigured())
}

func TestSynologyAutoSyncNotConfiguredWhenAllAlbumsDeselected(t *testing.T) {
	db := setupGateDB(t)
	svc, c := synologyGateService(t, db)

	mkConnAlbum(t, db, c, "42", model.AlbumKindReal, false)

	assert.False(t, svc.isAutoSyncConfigured())
}

func TestSynologyAutoSyncNotConfiguredWithoutCredentials(t *testing.T) {
	db := setupGateDB(t)
	svc, c := synologyGateService(t, db)
	mkConnAlbum(t, db, c, "42", model.AlbumKindReal, true)

	require.NoError(t, c.EncodeConfig(model.SynologyConfig{URL: "http://nas.local", Account: "user"}))
	require.NoError(t, NewConnectionStore(db).Update(&c))

	assert.False(t, svc.isAutoSyncConfigured())
}

// Two connections of one source keep separate pools: clearing one leaves the
// other's photos alone.
func TestClearSourcePhotosIsScopedToTheConnection(t *testing.T) {
	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Connection{}))
	a := mkConnection(t, db, model.SourceImmich, "A", model.ImmichConfig{})
	b := mkConnection(t, db, model.SourceImmich, "B", model.ImmichConfig{})
	aID, bID := a.ID, b.ID
	require.NoError(t, db.Create(&model.Image{Source: model.SourceImmich, ConnectionID: &aID, ExternalID: "x"}).Error)
	require.NoError(t, db.Create(&model.Image{Source: model.SourceImmich, ConnectionID: &bID, ExternalID: "x"}).Error)

	require.NoError(t, clearSourcePhotos(db, sourceScope{Source: model.SourceImmich, ConnectionID: a.ID}))

	var n int64
	db.Model(&model.Image{}).Where("connection_id = ?", a.ID).Count(&n)
	assert.Zero(t, n, "connection A must be empty")
	db.Model(&model.Image{}).Where("connection_id = ?", b.ID).Count(&n)
	assert.EqualValues(t, 1, n, "connection B must keep its photo")

	cnt, err := sourcePhotoCount(db, sourceScope{Source: model.SourceImmich, ConnectionID: b.ID})
	require.NoError(t, err)
	assert.EqualValues(t, 1, cnt)
}
