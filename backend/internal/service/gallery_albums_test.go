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

var galleryTestDBCounter atomic.Int64

func setupGalleryDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := galleryTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:gallery_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Image{}, &model.Album{}, &model.ImageAlbumMembership{},
		&model.Device{}, &model.DeviceAlbumMapping{}))
	return db
}

func galleryPhoto(t *testing.T, db *gorm.DB, path string) model.Image {
	t.Helper()
	img := model.Image{Source: model.SourceGallery, FilePath: path, Width: 4, Height: 3, Orientation: "landscape", Status: "pending"}
	require.NoError(t, db.Create(&img).Error)
	return img
}

func albumOf(t *testing.T, db *gorm.DB, imageID uint) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&model.ImageAlbumMembership{}).Where("image_id = ?", imageID).Pluck("album_id", &ids).Error)
	return ids
}

// Gallery albums are plain album rows of the gallery source: no connection,
// sync-enabled so the pickers list them, with a name that can change.
func TestGalleryAlbumCreateRename(t *testing.T) {
	db := setupGalleryDB(t)

	a, err := CreateGalleryAlbum(db, "  Family  ")
	require.NoError(t, err)
	assert.Equal(t, "Family", a.Name)
	assert.Equal(t, model.SourceGallery, a.Source)
	assert.Nil(t, a.ConnectionID)
	assert.True(t, a.SyncEnabled)
	assert.Equal(t, model.AlbumKindReal, a.Kind)

	b, err := CreateGalleryAlbum(db, "Art")
	require.NoError(t, err)
	assert.NotEqual(t, a.ExternalID, b.ExternalID)

	_, err = CreateGalleryAlbum(db, "   ")
	require.Error(t, err)

	renamed, err := RenameGalleryAlbum(db, a.ID, "Kids")
	require.NoError(t, err)
	assert.Equal(t, "Kids", renamed.Name)
	got, err := GalleryAlbum(db, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "Kids", got.Name)

	// An Immich album is not a gallery album, whatever its id.
	im := model.Album{Source: model.SourceImmich, ExternalID: "x", Name: "Remote"}
	require.NoError(t, db.Create(&im).Error)
	_, err = GalleryAlbum(db, im.ID)
	assert.ErrorIs(t, err, ErrGalleryAlbumNotFound)
	_, err = RenameGalleryAlbum(db, im.ID, "Nope")
	assert.ErrorIs(t, err, ErrGalleryAlbumNotFound)
}

// A gallery photo sits in at most one album; filing it elsewhere moves it,
// and nil unfiles it.
func TestGalleryPhotoAlbumIsExclusive(t *testing.T) {
	db := setupGalleryDB(t)
	a, _ := CreateGalleryAlbum(db, "A")
	b, _ := CreateGalleryAlbum(db, "B")
	p := galleryPhoto(t, db, "/p/1.jpg")

	require.NoError(t, SetGalleryPhotoAlbum(db, p.ID, &a.ID))
	assert.Equal(t, []uint{a.ID}, albumOf(t, db, p.ID))

	require.NoError(t, SetGalleryPhotoAlbum(db, p.ID, &b.ID))
	assert.Equal(t, []uint{b.ID}, albumOf(t, db, p.ID))

	require.NoError(t, SetGalleryPhotoAlbum(db, p.ID, nil))
	assert.Empty(t, albumOf(t, db, p.ID))

	// Only gallery photos and gallery albums take part.
	im := model.Image{Source: model.SourceImmich, ExternalID: "i1", FilePath: "x"}
	require.NoError(t, db.Create(&im).Error)
	assert.Error(t, SetGalleryPhotoAlbum(db, im.ID, &a.ID))
	remote := model.Album{Source: model.SourceImmich, ExternalID: "r", Name: "Remote"}
	require.NoError(t, db.Create(&remote).Error)
	assert.ErrorIs(t, SetGalleryPhotoAlbum(db, p.ID, &remote.ID), ErrGalleryAlbumNotFound)
}

// Deleting an album unfiles its photos; deleting it with its photos removes
// their rows and reports them so the files can go too. An album a frame is
// limited to is refused until the frame is moved: the frame would otherwise
// widen to the whole gallery.
func TestGalleryAlbumDelete(t *testing.T) {
	db := setupGalleryDB(t)
	a, _ := CreateGalleryAlbum(db, "A")
	b, _ := CreateGalleryAlbum(db, "B")
	p1 := galleryPhoto(t, db, "/p/1.jpg")
	p2 := galleryPhoto(t, db, "/p/2.jpg")
	p3 := galleryPhoto(t, db, "/p/3.jpg")
	require.NoError(t, SetGalleryPhotoAlbum(db, p1.ID, &a.ID))
	require.NoError(t, SetGalleryPhotoAlbum(db, p2.ID, &a.ID))
	require.NoError(t, SetGalleryPhotoAlbum(db, p3.ID, &b.ID))
	hall := model.Device{Name: "Hall", Host: "10.0.0.7", Source: model.SourceGallery}
	require.NoError(t, db.Create(&hall).Error)
	require.NoError(t, db.Create(&model.DeviceAlbumMapping{DeviceID: hall.ID, AlbumID: a.ID}).Error)
	require.NoError(t, db.Create(&model.DeviceAlbumMapping{DeviceID: hall.ID, AlbumID: b.ID}).Error)
	// A frame on another source only remembers its gallery selection; it
	// does not hold the album, and the deletion clears the memory.
	study := model.Device{Name: "Study", Host: "10.0.0.8", Source: model.SourceImmich}
	require.NoError(t, db.Create(&study).Error)
	require.NoError(t, db.Create(&model.DeviceAlbumMapping{DeviceID: study.ID, AlbumID: a.ID}).Error)

	_, err := DeleteGalleryAlbum(db, a.ID, false)
	var inUse *InUseError
	require.ErrorAs(t, err, &inUse)
	assert.Equal(t, []string{"Hall"}, inUse.DeviceNames())
	assert.Contains(t, err.Error(), `the album "A" is used by 1 frame: Hall`)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, DeviceAlbumIDs(db, hall.ID, model.SourceGallery), "the refusal changes nothing")
	_, err = DeleteGalleryAlbum(db, a.ID, true)
	require.ErrorAs(t, err, &inUse, "with photos too")

	// The frame moved off A: now it goes.
	require.NoError(t, db.Where("device_id = ? AND album_id = ?", hall.ID, a.ID).Delete(&model.DeviceAlbumMapping{}).Error)
	removed, err := DeleteGalleryAlbum(db, a.ID, false)
	require.NoError(t, err)
	assert.Empty(t, removed)
	var n int64
	db.Model(&model.Image{}).Where("source = ?", model.SourceGallery).Count(&n)
	assert.EqualValues(t, 3, n, "photos stay when the album goes")
	assert.Empty(t, albumOf(t, db, p1.ID))
	assert.Equal(t, []uint{b.ID}, DeviceAlbumIDs(db, hall.ID, model.SourceGallery))
	assert.Empty(t, DeviceAlbumIDs(db, study.ID, model.SourceGallery), "the inert selection is cleared with the album")

	require.NoError(t, db.Where("device_id = ?", hall.ID).Delete(&model.DeviceAlbumMapping{}).Error)
	removed, err = DeleteGalleryAlbum(db, b.ID, true)
	require.NoError(t, err)
	require.Len(t, removed, 1)
	assert.Equal(t, "/p/3.jpg", removed[0].FilePath)
	db.Model(&model.Image{}).Where("source = ?", model.SourceGallery).Count(&n)
	assert.EqualValues(t, 2, n)

	_, err = DeleteGalleryAlbum(db, b.ID, false)
	assert.ErrorIs(t, err, ErrGalleryAlbumNotFound)
}

// A frame bound to gallery albums is served from them only; one without a
// binding gets the whole gallery, unfiled photos included.
func TestGalleryPickHonoursAlbumBinding(t *testing.T) {
	db := setupGalleryDB(t)
	a, _ := CreateGalleryAlbum(db, "A")
	inA := galleryPhoto(t, db, "/p/a.jpg")
	loose := galleryPhoto(t, db, "/p/loose.jpg")
	require.NoError(t, SetGalleryPhotoAlbum(db, inA.ID, &a.ID))
	require.NoError(t, db.Create(&model.DeviceAlbumMapping{DeviceID: 1, AlbumID: a.ID}).Error)

	bound := DeviceAlbumIDs(db, 1, model.SourceGallery)
	assert.Equal(t, []uint{a.ID}, bound)
	for i := 0; i < 10; i++ {
		got, err := PickRandomDBPhotoForAlbums(db, model.SourceGallery, "", bound, nil)
		require.NoError(t, err)
		assert.Equal(t, inA.ID, got.ID)
	}
	// Excluding the only album photo leaves nothing: no fallback to loose ones.
	_, err := PickRandomDBPhotoForAlbums(db, model.SourceGallery, "", bound, []uint{inA.ID})
	assert.Error(t, err)

	seen := map[uint]bool{}
	for i := 0; i < 30 && len(seen) < 2; i++ {
		got, err := PickRandomDBPhotoForAlbums(db, model.SourceGallery, "", DeviceAlbumIDs(db, 2, model.SourceGallery), nil)
		require.NoError(t, err)
		seen[got.ID] = true
	}
	assert.True(t, seen[inA.ID] && seen[loose.ID], "unbound frame draws from the whole gallery: %v", seen)
}
