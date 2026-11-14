package service

// Shared, source-parameterized helpers for the DB-backed photo sources
// (Immich, Synology). They factor out logic that was duplicated verbatim
// between immich.go and synology.go, differing only by the source constant.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// sourceScope names one pool of albums and images: a connection (one Immich
// server, one NAS account), or, for sources without connections (topic
// sources), the rows of a source type with no connection. Every shared helper
// filters on it, so clearing or syncing one connection never touches another
// connection of the same type.
type sourceScope struct {
	Source       string
	ConnectionID uint // 0 = the source's connection-less rows
}

// where narrows a query on albums or images to the scope.
func (sc sourceScope) where(q *gorm.DB) *gorm.DB {
	if sc.ConnectionID != 0 {
		return q.Where("connection_id = ?", sc.ConnectionID)
	}
	return q.Where("source = ? AND connection_id IS NULL", sc.Source)
}

// connectionRef is the value to store in a new row's connection_id.
func (sc sourceScope) connectionRef() *uint {
	if sc.ConnectionID == 0 {
		return nil
	}
	id := sc.ConnectionID
	return &id
}

func (sc sourceScope) String() string {
	if sc.ConnectionID != 0 {
		return fmt.Sprintf("%s#%d", sc.Source, sc.ConnectionID)
	}
	return sc.Source
}

// thumbnailDir is where gallery thumbnails are cached as <imageID>.jpg. Set
// once at startup via SetThumbnailDir; when empty (e.g. unit tests without a
// data dir) the thumbnail file GC is a no-op.
var thumbnailDir string

// SetThumbnailDir configures the cached-thumbnail directory swept by the
// orphan-thumbnail GC after sync-driven image deletions.
func SetThumbnailDir(dir string) { thumbnailDir = dir }

// clearSourcePhotos hard-deletes all of the source's image rows. Their album
// memberships drop via ON DELETE CASCADE (this is an Unscoped hard delete, so
// the cascade fires).
func clearSourcePhotos(db *gorm.DB, sc sourceScope) error {
	if err := sc.where(db.Unscoped().Model(&model.Image{})).
		Delete(&model.Image{}).Error; err != nil {
		return err
	}
	gcOrphanThumbnails(db)
	log.Printf("Cleared all %s photos from database", sc)
	return nil
}

// gcOrphanImagesForSource removes the source's image rows that are no longer a
// member of any album (their album was disabled or they were removed upstream),
// then sweeps cached thumbnail files the deleted rows leave behind.
func gcOrphanImagesForSource(db *gorm.DB, sc sourceScope) {
	sub := db.Model(&model.ImageAlbumMembership{}).Select("image_id")
	if err := sc.where(db.Unscoped().Model(&model.Image{})).
		Where("id NOT IN (?)", sub).
		Delete(&model.Image{}).Error; err != nil {
		log.Printf("[%s] gc orphan images: %v", sc, err)
	}
	gcOrphanThumbnails(db)
}

// pruneDisabledAlbumMemberships drops memberships owned by the source's
// sync-disabled albums so gcOrphanImagesForSource can sweep their images.
// Without it, unchecking an album kept its photos in the DB (and showing in
// the gallery / rotating onto devices) forever, because the orphan GC only
// removes images with no memberships at all.
func pruneDisabledAlbumMemberships(db *gorm.DB, sc sourceScope) {
	sub := sc.where(db.Model(&model.Album{})).Select("id").Where("sync_enabled = ?", false)
	if err := db.Where("album_id IN (?)", sub).
		Delete(&model.ImageAlbumMembership{}).Error; err != nil {
		log.Printf("[%s] prune disabled-album memberships: %v", sc, err)
	}
}

// gcOrphanThumbnails deletes cached thumbnail files whose image row is gone.
// Sync-driven deletions (membership prune + orphan GC, source clears) happen
// in bulk SQL and don't know which files belonged to the deleted rows, so
// this sweeps the whole cache directory instead; that also reclaims files
// leaked before this GC existed. Removing stale files matters beyond disk
// space: SQLite reuses row ids, so a leftover <id>.jpg could be served as the
// thumbnail of a future, different image.
func gcOrphanThumbnails(db *gorm.DB) {
	if thumbnailDir == "" {
		return
	}
	entries, err := os.ReadDir(thumbnailDir)
	if err != nil {
		return
	}
	nameByID := map[uint64]string{}
	var ids []uint64
	for _, e := range entries {
		idStr, ok := strings.CutSuffix(e.Name(), ".jpg")
		if !ok {
			continue
		}
		id, perr := strconv.ParseUint(idStr, 10, 64)
		if perr != nil {
			continue
		}
		nameByID[id] = e.Name()
		ids = append(ids, id)
	}
	// Batch the lookups to stay under SQLite's bound-variable limit.
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		var alive []uint64
		if err := db.Model(&model.Image{}).Where("id IN ?", ids[start:end]).
			Pluck("id", &alive).Error; err != nil {
			return // on DB error, keep files: orphan files beat missing thumbnails
		}
		for _, id := range alive {
			delete(nameByID, id)
		}
	}
	for _, name := range nameByID {
		os.Remove(filepath.Join(thumbnailDir, name))
	}
}

// sourcePhotoCount returns the number of (non-deleted) images for a source.
func sourcePhotoCount(db *gorm.DB, sc sourceScope) (int64, error) {
	var count int64
	if err := sc.where(db.Model(&model.Image{})).Count(&count).Error; err != nil {
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
