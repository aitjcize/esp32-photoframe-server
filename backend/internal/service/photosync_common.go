package service

// Shared, source-parameterized helpers for the DB-backed photo sources
// (Immich, Synology). They factor out logic that was duplicated verbatim
// between immich.go and synology.go, differing only by the source constant.

import (
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// clearSourcePhotos hard-deletes all of the source's image rows. Their album
// memberships drop via ON DELETE CASCADE (this is an Unscoped hard delete, so
// the cascade fires).
func clearSourcePhotos(db *gorm.DB, source string) error {
	if err := db.Unscoped().Where("source = ?", source).
		Delete(&model.Image{}).Error; err != nil {
		return err
	}
	log.Printf("Cleared all %s photos from database", source)
	return nil
}

// gcOrphanImagesForSource removes the source's image rows that are no longer a
// member of any album (their album was disabled or they were removed upstream).
func gcOrphanImagesForSource(db *gorm.DB, source string) {
	sub := db.Model(&model.ImageAlbumMembership{}).Select("image_id")
	if err := db.Unscoped().
		Where("source = ? AND id NOT IN (?)", source, sub).
		Delete(&model.Image{}).Error; err != nil {
		log.Printf("[%s] gc orphan images: %v", source, err)
	}
}

// sourcePhotoCount returns the number of (non-deleted) images for a source.
func sourcePhotoCount(db *gorm.DB, source string) (int64, error) {
	var count int64
	if err := db.Model(&model.Image{}).Where("source = ?", source).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// parseAutoSyncConfig reads the enabled flag and interval (minutes, default 60)
// for a source's auto-sync from settings.
func parseAutoSyncConfig(settings *SettingsService, enabledKey, intervalKey string) (bool, time.Duration) {
	enabledStr, _ := settings.Get(enabledKey)
	enabled := strings.EqualFold(enabledStr, "true")

	minutes := 60
	if intervalStr, err := settings.Get(intervalKey); err == nil {
		if parsed, perr := strconv.Atoi(intervalStr); perr == nil && parsed > 0 {
			minutes = parsed
		}
	}
	return enabled, time.Duration(minutes) * time.Minute
}
