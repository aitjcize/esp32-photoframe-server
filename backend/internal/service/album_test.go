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
