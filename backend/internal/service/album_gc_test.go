package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

func mkMembership(t *testing.T, db *gorm.DB, img model.Image, album model.Album) {
	t.Helper()
	require.NoError(t, db.Create(&model.ImageAlbumMembership{ImageID: img.ID, AlbumID: album.ID}).Error)
}

func imageExists(db *gorm.DB, id uint) bool {
	var n int64
	db.Model(&model.Image{}).Where("id = ?", id).Count(&n)
	return n > 0
}

// Deselecting an album via the shared SetSyncAlbums must remove its images
// immediately, unless they also belong to a still-enabled album.
func TestSetSyncAlbumsRemovesDeselectedImages(t *testing.T) {
	db := setupAlbumDB(t)
	albumA := mkAlbum(t, db, "album-a")
	albumB := mkAlbum(t, db, "album-b")

	onlyA := mkImage(t, db, "only-a")
	onlyB := mkImage(t, db, "only-b")
	inBoth := mkImage(t, db, "in-both")
	mkMembership(t, db, onlyA, albumA)
	mkMembership(t, db, onlyB, albumB)
	mkMembership(t, db, inBoth, albumA)
	mkMembership(t, db, inBoth, albumB)

	// Keep only album A checked.
	require.NoError(t, SetSyncAlbums(db, model.SourceImmich, []RemoteAlbum{
		{ExternalID: "album-a", Name: "A"},
	}))

	assert.True(t, imageExists(db, onlyA.ID), "image in the kept album must survive")
	assert.False(t, imageExists(db, onlyB.ID), "image only in the deselected album must be removed")
	assert.True(t, imageExists(db, inBoth.ID), "image shared with a kept album must survive")

	var mems int64
	db.Model(&model.ImageAlbumMembership{}).Where("album_id = ?", albumB.ID).Count(&mems)
	assert.Zero(t, mems, "deselected album must have no memberships left")
}

type fakeAlbumSource struct {
	assets map[string][]RemoteAsset // by album external id
	errs   map[string]error         // by album external id
}

func (f *fakeAlbumSource) Source() string                           { return model.SourceImmich }
func (f *fakeAlbumSource) ListRemoteAlbums() ([]RemoteAlbum, error) { return nil, nil }
func (f *fakeAlbumSource) FetchAlbumAssets(a model.Album) ([]RemoteAsset, error) {
	if err := f.errs[a.ExternalID]; err != nil {
		return nil, err
	}
	return f.assets[a.ExternalID], nil
}

// A resync must clean up after albums that were disabled through any path
// (e.g. Immich virtual-album toggles), not just the shared SetSyncAlbums.
func TestSyncAlbumSourceRemovesDisabledAlbumImages(t *testing.T) {
	db := setupAlbumDB(t)
	enabled := mkAlbum(t, db, "enabled")
	disabled := mkAlbum(t, db, "disabled")
	require.NoError(t, db.Model(&model.Album{}).Where("id = ?", disabled.ID).
		Update("sync_enabled", false).Error)

	stale := mkImage(t, db, "stale")
	mkMembership(t, db, stale, disabled)

	src := &fakeAlbumSource{assets: map[string][]RemoteAsset{
		"enabled": {{ExternalID: "fresh", Width: 100, Height: 50}},
	}}
	newCount, err := SyncAlbumSource(db, src)
	require.NoError(t, err)
	assert.Equal(t, 1, newCount)

	assert.False(t, imageExists(db, stale.ID), "disabled album's image must be removed on resync")
	var fresh model.Image
	require.NoError(t, db.Where("external_id = ?", "fresh").First(&fresh).Error)
	_ = enabled
}

// A failing album must not stop the sync of healthy albums, but the failure
// must show up in the returned error so the UI can surface it instead of
// silently reporting "0 new photos" — see issue #44.
func TestSyncAlbumSourceReportsAlbumFailures(t *testing.T) {
	db := setupAlbumDB(t)
	mkAlbum(t, db, "good")
	mkAlbum(t, db, "bad")

	src := &fakeAlbumSource{
		assets: map[string][]RemoteAsset{
			"good": {{ExternalID: "fresh", Width: 100, Height: 50}},
		},
		errs: map[string]error{"bad": fmt.Errorf("memories returned status 400")},
	}
	newCount, err := SyncAlbumSource(db, src)
	assert.Equal(t, 1, newCount, "healthy album must still be synced")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 2 album(s) failed")
	assert.Contains(t, err.Error(), "status 400")
}

// Unchecking every album and resyncing must remove all of the source's images.
func TestSyncAlbumSourceAllAlbumsDisabled(t *testing.T) {
	db := setupAlbumDB(t)
	album := mkAlbum(t, db, "album")
	require.NoError(t, db.Model(&model.Album{}).Where("id = ?", album.ID).
		Update("sync_enabled", false).Error)
	img := mkImage(t, db, "asset")
	mkMembership(t, db, img, album)

	_, err := SyncAlbumSource(db, &fakeAlbumSource{})
	require.NoError(t, err)
	assert.False(t, imageExists(db, img.ID))
}

// The thumbnail GC must delete cached files whose image row is gone and leave
// everything else (live thumbnails, unrelated files) alone.
func TestGcOrphanThumbnails(t *testing.T) {
	db := setupAlbumDB(t)
	dir := t.TempDir()
	SetThumbnailDir(dir)
	t.Cleanup(func() { SetThumbnailDir("") })

	album := mkAlbum(t, db, "album")
	img := mkImage(t, db, "asset")
	mkMembership(t, db, img, album)

	livePath := filepath.Join(dir, fmt.Sprintf("%d.jpg", img.ID))
	orphanPath := filepath.Join(dir, "99999.jpg")
	otherPath := filepath.Join(dir, "notes.txt")
	for _, p := range []string{livePath, orphanPath, otherPath} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	gcOrphanThumbnails(db)

	assert.FileExists(t, livePath, "live image's thumbnail must survive")
	assert.NoFileExists(t, orphanPath, "orphaned thumbnail must be removed")
	assert.FileExists(t, otherPath, "non-thumbnail files must be untouched")
}
