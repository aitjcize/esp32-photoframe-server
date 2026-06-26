package service

// Album-scoped photo selection shared by the DB-backed sources. Kept separate
// from photosource_helpers.go so the album feature stays self-contained.

import (
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// DeviceAlbumIDs returns the album row IDs a device is bound to for the given
// source. An empty result means "no per-device selection" — callers fall back
// to the whole-source pool (preserving legacy behavior).
func DeviceAlbumIDs(db *gorm.DB, deviceID uint, source string) []uint {
	var ids []uint
	db.Model(&model.DeviceAlbumMapping{}).
		Joins("JOIN albums ON albums.id = device_album_mappings.album_id").
		Where("device_album_mappings.device_id = ? AND albums.source = ?", deviceID, source).
		Pluck("device_album_mappings.album_id", &ids)
	return ids
}

// PickRandomDBPhotoForAlbums is PickRandomDBPhoto restricted to images that are
// members of the given album IDs. With no album IDs it falls back to the
// whole-source pool, so devices without a binding keep working as before.
func PickRandomDBPhotoForAlbums(
	db *gorm.DB, source, orientationFilter string, albumIDs, excludeIDs []uint,
) (model.Image, error) {
	if len(albumIDs) == 0 {
		return PickRandomDBPhoto(db, source, orientationFilter, excludeIDs)
	}

	query := db.Model(&model.Image{}).
		Joins("JOIN image_album_memberships m ON m.image_id = images.id").
		Where("images.source = ?", source).
		Where("m.album_id IN ?", albumIDs).
		Order("RANDOM()")
	if len(excludeIDs) > 0 {
		query = query.Where("images.id NOT IN ?", excludeIDs)
	}
	if orientationFilter != "" {
		query = query.Where("images.orientation IN ?", []string{orientationFilter, "auto"})
	}

	var item model.Image
	err := query.First(&item).Error
	return item, err
}
