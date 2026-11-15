package service

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

var albumTestDBCounter atomic.Int64

func setupAlbumDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := albumTestDBCounter.Add(1)
	dsn := fmt.Sprintf("file:album_test_%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Image{}, &model.Album{},
		&model.ImageAlbumMembership{}, &model.DeviceAlbumMapping{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func mkImage(t *testing.T, db *gorm.DB, assetID string) model.Image {
	t.Helper()
	img := model.Image{ExternalID: assetID, Source: model.SourceImmich, Status: "pending"}
	if err := db.Create(&img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	return img
}

func mkAlbum(t *testing.T, db *gorm.DB, ext string) model.Album {
	t.Helper()
	a := model.Album{Source: model.SourceImmich, ExternalID: ext, Kind: model.AlbumKindReal, SyncEnabled: true}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create album: %v", err)
	}
	return a
}

func TestDeviceAlbumIDsAndAlbumScopedPick(t *testing.T) {
	db := setupAlbumDB(t)

	albumA := mkAlbum(t, db, "album-a")
	albumB := mkAlbum(t, db, "album-b")

	imgA := mkImage(t, db, "asset-a")
	imgB := mkImage(t, db, "asset-b")
	db.Create(&model.ImageAlbumMembership{ImageID: imgA.ID, AlbumID: albumA.ID})
	db.Create(&model.ImageAlbumMembership{ImageID: imgB.ID, AlbumID: albumB.ID})

	// device 1 -> album A, device 2 -> album B
	db.Create(&model.DeviceAlbumMapping{DeviceID: 1, AlbumID: albumA.ID})
	db.Create(&model.DeviceAlbumMapping{DeviceID: 2, AlbumID: albumB.ID})

	assert.ElementsMatch(t, []uint{albumA.ID}, DeviceAlbumIDs(db, 1, model.SourceImmich))
	assert.ElementsMatch(t, []uint{albumB.ID}, DeviceAlbumIDs(db, 2, model.SourceImmich))
	assert.Empty(t, DeviceAlbumIDs(db, 3, model.SourceImmich)) // unbound device

	// Each device sees only its album's photo.
	got, err := PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumA.ID}, nil)
	assert.NoError(t, err)
	assert.Equal(t, imgA.ID, got.ID)

	got, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumB.ID}, nil)
	assert.NoError(t, err)
	assert.Equal(t, imgB.ID, got.ID)

	// Excluding the only member of an album yields no photo (no cross-album leak).
	_, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumA.ID}, []uint{imgA.ID})
	assert.Error(t, err)

	// No album binding -> whole-source pool (legacy fallback).
	_, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", nil, nil)
	assert.NoError(t, err)
}

// A frame bound to one connection draws only from that connection's photos,
// narrowed further by its album bindings of that connection; album bindings
// left over from another connection do not empty its pool. An unbound frame
// still draws from every connection.
func TestPickRandomDBPhotoForDeviceHonoursTheConnectionBinding(t *testing.T) {
	db := setupAlbumDB(t)
	if err := db.AutoMigrate(&model.Connection{}); err != nil {
		t.Fatal(err)
	}
	home := model.Connection{Source: model.SourceImmich, Name: "Home", Enabled: true}
	nas := model.Connection{Source: model.SourceImmich, Name: "Parents", Enabled: true}
	db.Create(&home)
	db.Create(&nas)
	conn := func(c model.Connection) model.Image {
		img := model.Image{ExternalID: c.Name, Source: model.SourceImmich, Status: "pending", ConnectionID: &c.ID}
		db.Create(&img)
		return img
	}
	imgHome := conn(home)
	imgHome2 := model.Image{ExternalID: "home-2", Source: model.SourceImmich, Status: "pending", ConnectionID: &home.ID}
	db.Create(&imgHome2)
	imgNas := conn(nas)
	albumHome := model.Album{Source: model.SourceImmich, ExternalID: "trips", Kind: model.AlbumKindReal, SyncEnabled: true, ConnectionID: &home.ID}
	albumNas := model.Album{Source: model.SourceImmich, ExternalID: "family", Kind: model.AlbumKindReal, SyncEnabled: true, ConnectionID: &nas.ID}
	db.Create(&albumHome)
	db.Create(&albumNas)
	db.Create(&model.ImageAlbumMembership{ImageID: imgHome.ID, AlbumID: albumHome.ID})
	db.Create(&model.ImageAlbumMembership{ImageID: imgNas.ID, AlbumID: albumNas.ID})

	// Unbound: any connection.
	seen := map[uint]bool{}
	for i := 0; i < 40; i++ {
		got, err := PickRandomDBPhotoForDevice(db, model.SourceImmich, &model.Device{ID: 1}, "", nil)
		assert.NoError(t, err)
		seen[got.ID] = true
	}
	assert.True(t, seen[imgNas.ID] && (seen[imgHome.ID] || seen[imgHome2.ID]), "unbound frame draws from both connections: %v", seen)

	// Bound to the NAS: only its photo, even with no album bound.
	bound := &model.Device{ID: 2, ImageConnectionID: &nas.ID}
	got, err := PickRandomDBPhotoForDevice(db, model.SourceImmich, bound, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, imgNas.ID, got.ID)
	_, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, bound, "", []uint{imgNas.ID})
	assert.Error(t, err, "the other connection's photos are not a fallback")

	// Bound to Home with a Home album: only that album's photo. A stale
	// binding to the NAS album is ignored, not an empty intersection.
	db.Create(&model.DeviceAlbumMapping{DeviceID: 3, AlbumID: albumHome.ID})
	db.Create(&model.DeviceAlbumMapping{DeviceID: 3, AlbumID: albumNas.ID})
	homeFrame := &model.Device{ID: 3, ImageConnectionID: &home.ID}
	for i := 0; i < 10; i++ {
		got, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, homeFrame, "", nil)
		assert.NoError(t, err)
		assert.Equal(t, imgHome.ID, got.ID)
	}
	// Only the stale binding: the whole connection, not nothing.
	db.Create(&model.DeviceAlbumMapping{DeviceID: 4, AlbumID: albumNas.ID})
	got, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, &model.Device{ID: 4, ImageConnectionID: &home.ID}, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, &home.ID, got.ConnectionID)

	// The handler's check: a binding must name a connection of the frame's
	// source; built-in sources and 0 clear it.
	zero := uint(0)
	for _, tc := range []struct {
		source string
		id     *uint
		want   *uint
		bad    bool
	}{
		{model.SourceImmich, nil, nil, false},
		{model.SourceImmich, &zero, nil, false},
		{model.SourceImmich, &nas.ID, &nas.ID, false},
		{model.SourceGallery, &nas.ID, nil, false},
		{model.SourceSynologyPhotos, &nas.ID, nil, true},
		{model.SourceImmich, ptrUint(999), nil, true},
	} {
		got, err := ImageConnectionFor(db, tc.source, tc.id)
		if tc.bad {
			assert.Error(t, err, "%+v", tc)
			continue
		}
		assert.NoError(t, err, "%+v", tc)
		assert.Equal(t, tc.want, got, "%+v", tc)
	}
}

func ptrUint(v uint) *uint { return &v }
