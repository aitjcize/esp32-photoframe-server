package service

// Album-scoped photo selection shared by the DB-backed sources. Kept separate
// from photosource_helpers.go so the album feature stays self-contained.

import (
	"errors"
	"fmt"

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

// PickRandomDBPhotoForDevice picks a photo of source for one frame: from the
// connection it is bound to (none: every connection of the source) and,
// within that, from the albums it selected there (none: that whole pool). A
// nil device gets the whole pool. Album selections are kept per connection
// so a frame can be switched away and back; the selections of connections
// the frame is not on play no part here.
func PickRandomDBPhotoForDevice(
	db *gorm.DB, source string, device *model.Device, orientationFilter string, excludeIDs []uint,
) (model.Image, error) {
	if device == nil {
		return PickRandomDBPhoto(db, source, orientationFilter, excludeIDs)
	}
	if device.ImageConnectionID == nil {
		return PickRandomDBPhotoForAlbums(db, source, orientationFilter,
			DeviceAlbumIDs(db, device.ID, source), excludeIDs)
	}
	conn := *device.ImageConnectionID
	var albumIDs []uint
	db.Model(&model.DeviceAlbumMapping{}).
		Joins("JOIN albums ON albums.id = device_album_mappings.album_id").
		Where("device_album_mappings.device_id = ? AND albums.source = ? AND albums.connection_id = ?",
			device.ID, source, conn).
		Pluck("device_album_mappings.album_id", &albumIDs)
	return pickRandomDBPhoto(db.Where("images.connection_id = ?", conn), source, orientationFilter, albumIDs, excludeIDs)
}

// PickRandomDBPhotoForAlbums is PickRandomDBPhoto restricted to images that are
// members of the given album IDs. With no album IDs it falls back to the
// whole-source pool, so devices without a binding keep working as before.
func PickRandomDBPhotoForAlbums(
	db *gorm.DB, source, orientationFilter string, albumIDs, excludeIDs []uint,
) (model.Image, error) {
	return pickRandomDBPhoto(db, source, orientationFilter, albumIDs, excludeIDs)
}

// pickRandomDBPhoto is the shared query: db may already carry a filter on
// the images table (the connection a frame is bound to).
func pickRandomDBPhoto(
	db *gorm.DB, source, orientationFilter string, albumIDs, excludeIDs []uint,
) (model.Image, error) {
	query := db.Model(&model.Image{}).Where("images.source = ?", source).Order("RANDOM()")
	if len(albumIDs) > 0 {
		query = query.Joins("JOIN image_album_memberships m ON m.image_id = images.id").
			Where("m.album_id IN ?", albumIDs)
	}
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

// ImageConnectionFor checks a frame's requested connection binding against
// its source: a built-in source takes none, and a connection-backed one takes
// nil ("any connection") or the id of a connection of that very source. It
// returns the value to store or an error fit for a 400.
func ImageConnectionFor(db *gorm.DB, source string, id *uint) (*uint, error) {
	if id == nil || *id == 0 || !isConnectionSource(source) {
		return nil, nil
	}
	var conn model.Connection
	if err := db.First(&conn, *id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("connection %d not found", *id)
		}
		return nil, err
	}
	if conn.Source != source {
		return nil, fmt.Errorf("connection %d is a %s connection, not %s", *id, conn.Source, source)
	}
	return id, nil
}

func isConnectionSource(source string) bool {
	for _, s := range model.ConnectionSources {
		if s == source {
			return true
		}
	}
	return false
}
