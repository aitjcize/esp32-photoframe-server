package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// Gallery albums are folders the user makes inside the local gallery. They
// are ordinary rows of the albums table (source "gallery", no connection,
// always sync-enabled), so the per-frame album binding, the gallery's album
// chips and the album filter of the photo list work on them unchanged. A
// gallery photo is in at most one album; photos in none are "unfiled" and
// still part of the whole-gallery pool.

// ErrGalleryAlbumNotFound is returned for an id that is not a gallery album.
var ErrGalleryAlbumNotFound = errors.New("gallery album not found")

var galleryScope = sourceScope{Source: model.SourceGallery}

// GalleryAlbum loads one gallery album.
func GalleryAlbum(db *gorm.DB, id uint) (model.Album, error) {
	var a model.Album
	err := galleryScope.where(db.Model(&model.Album{})).Where("id = ?", id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a, ErrGalleryAlbumNotFound
	}
	return a, err
}

// CreateGalleryAlbum adds an empty album. Names are trimmed and must not be
// blank; two albums may share a name (the id tells them apart).
func CreateGalleryAlbum(db *gorm.DB, name string) (model.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Album{}, errors.New("album name is required")
	}
	nonce, err := randomNonce()
	if err != nil {
		return model.Album{}, err
	}
	a := model.Album{
		Source:      model.SourceGallery,
		ExternalID:  "gallery-" + nonce,
		Name:        name,
		Kind:        model.AlbumKindReal,
		SyncEnabled: true,
	}
	if err := db.Create(&a).Error; err != nil {
		return model.Album{}, err
	}
	return a, nil
}

// RenameGalleryAlbum changes an album's name.
func RenameGalleryAlbum(db *gorm.DB, id uint, name string) (model.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Album{}, errors.New("album name is required")
	}
	a, err := GalleryAlbum(db, id)
	if err != nil {
		return a, err
	}
	if err := db.Model(&model.Album{}).Where("id = ?", a.ID).Update("name", name).Error; err != nil {
		return a, err
	}
	a.Name = name
	return a, nil
}

// GalleryAlbumPhotos lists the photos in an album.
func GalleryAlbumPhotos(db *gorm.DB, albumID uint) ([]model.Image, error) {
	var items []model.Image
	err := db.Model(&model.Image{}).
		Joins("JOIN image_album_memberships m ON m.image_id = images.id").
		Where("m.album_id = ?", albumID).
		Find(&items).Error
	return items, err
}

// DeleteGalleryAlbum removes an album. Its photos stay in the gallery as
// unfiled unless withPhotos is set, in which case their rows go too and the
// removed images are returned so the caller can delete their files. Frames
// bound to the album lose that binding (ON DELETE CASCADE) and fall back to
// the rest of their selection or the whole gallery.
func DeleteGalleryAlbum(db *gorm.DB, id uint, withPhotos bool) ([]model.Image, error) {
	a, err := GalleryAlbum(db, id)
	if err != nil {
		return nil, err
	}
	// A frame limited to this album would otherwise widen to the whole
	// gallery; its owner moves it first.
	if users, err := FramesUsingAlbums(db, []uint{a.ID}); err != nil {
		return nil, err
	} else if len(users) > 0 {
		return nil, &InUseError{What: fmt.Sprintf("the album %q", a.Name), Devices: users}
	}
	var removed []model.Image
	if withPhotos {
		if removed, err = GalleryAlbumPhotos(db, a.ID); err != nil {
			return nil, err
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if withPhotos && len(removed) > 0 {
			ids := make([]uint, 0, len(removed))
			for _, im := range removed {
				ids = append(ids, im.ID)
			}
			if e := tx.Unscoped().Where("id IN ?", ids).Delete(&model.Image{}).Error; e != nil {
				return e
			}
		}
		// Memberships and remembered frame selections also cascade from the
		// album row, but not on a database opened without foreign keys; drop
		// them here. (No frame draws from the album: see above.)
		if e := tx.Where("album_id = ?", a.ID).Delete(&model.ImageAlbumMembership{}).Error; e != nil {
			return e
		}
		if e := tx.Where("album_id = ?", a.ID).Delete(&model.DeviceAlbumMapping{}).Error; e != nil {
			return e
		}
		return tx.Delete(&model.Album{}, a.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}

// SetGalleryPhotoAlbum files a gallery photo under one album, or under none
// when albumID is nil. Memberships in other gallery albums are dropped, so a
// photo is never in two folders at once.
func SetGalleryPhotoAlbum(db *gorm.DB, imageID uint, albumID *uint) error {
	var img model.Image
	if err := db.Where("id = ? AND source = ?", imageID, model.SourceGallery).First(&img).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("gallery photo %d not found", imageID)
		}
		return err
	}
	if albumID != nil {
		if _, err := GalleryAlbum(db, *albumID); err != nil {
			return err
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		sub := galleryScope.where(tx.Model(&model.Album{})).Select("id")
		if e := tx.Where("image_id = ? AND album_id IN (?)", imageID, sub).
			Delete(&model.ImageAlbumMembership{}).Error; e != nil {
			return e
		}
		if albumID == nil {
			return nil
		}
		return tx.Create(&model.ImageAlbumMembership{ImageID: imageID, AlbumID: *albumID}).Error
	})
}
