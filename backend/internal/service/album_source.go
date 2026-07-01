package service

import (
	"log"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"gorm.io/gorm"
)

// RemoteAsset is the source-agnostic upsert payload for one asset. Each source
// maps its API response to this; it is the ONLY per-source difference the shared
// album-sync engine sees.
type RemoteAsset struct {
	ExternalID string // stable source id (immich UUID, synology id, unsplash/pexels/artic id)
	// FilePath is how this source's downloader later fetches the bytes: a local
	// path for disk sources (immich/synology fetch by id instead), or the image
	// URL for URL-based sources (unsplash/pexels/artic).
	FilePath     string
	Caption      string // optional (e.g. ARTIC title / artist)
	Width        int
	Height       int
	Orientation  string // "landscape"|"portrait"|"auto"; if "" the engine derives it from w/h
	ThumbnailKey string // optional (synology cache key)
	PhotoTakenAt *time.Time
}

// RemoteAlbum is a source album (or, for search-topic sources, a topic) to sync.
type RemoteAlbum struct {
	ExternalID string
	Name       string
}

// AlbumSource syncs remote albums of assets into the local DB. The shared engine
// (SyncAlbumSource) drives it; a source implements just these three methods and
// the download, while the upsert/membership/prune/gc machinery is shared.
type AlbumSource interface {
	// Source returns the model.Source* constant this source owns.
	Source() string
	// ListRemoteAlbums is used for a best-effort album-name refresh during sync.
	// May return nil (e.g. topic sources where the album name is the topic).
	ListRemoteAlbums() ([]RemoteAlbum, error)
	// FetchAlbumAssets returns the current assets for one synced album.
	FetchAlbumAssets(album model.Album) ([]RemoteAsset, error)
}

// SyncAlbumSource runs the shared import loop: for each sync-enabled album, fetch
// its assets, upsert images + memberships (pruning removed ones), refresh the
// album name, then GC images orphaned from every album. Returns total new images.
// Generalizes what immich.go/synology.go's ImportPhotos each hand-rolled.
func SyncAlbumSource(db *gorm.DB, src AlbumSource) (int, error) {
	source := src.Source()

	var albums []model.Album
	if err := db.Where("source = ? AND sync_enabled = ?", source, true).Find(&albums).Error; err != nil {
		return 0, err
	}
	if len(albums) == 0 {
		log.Printf("%s sync: no albums enabled for sync", source)
		return 0, nil
	}

	// Best-effort name refresh for real albums.
	nameByExternalID := map[string]string{}
	if list, e := src.ListRemoteAlbums(); e == nil {
		for _, a := range list {
			nameByExternalID[a.ExternalID] = a.Name
		}
	}

	total := 0
	for _, album := range albums {
		assets, err := src.FetchAlbumAssets(album)
		if err != nil {
			log.Printf("%s: fetch album %q (%s) failed: %v", source, album.Name, album.ExternalID, err)
			continue // leave the album's prior state + count untouched
		}
		newCount, _, err := upsertAlbumAssets(db, source, album.ID, assets)
		if err != nil {
			log.Printf("%s: import album %q (%s) failed: %v", source, album.Name, album.ExternalID, err)
			continue
		}
		total += newCount

		updates := map[string]interface{}{"updated_at": time.Now()}
		if album.Kind == model.AlbumKindReal {
			if n := nameByExternalID[album.ExternalID]; n != "" {
				updates["name"] = n
			}
		}
		if err := db.Model(&model.Album{}).Where("id = ?", album.ID).Updates(updates).Error; err != nil {
			log.Printf("%s: update album %d (%q): %v", source, album.ID, album.Name, err)
		}
	}

	gcOrphanImagesForSource(db, source)
	log.Printf("%s sync complete: %d new photos across %d album(s)", source, total, len(albums))
	return total, nil
}

// upsertAlbumAssets upserts image rows + (asset, album) membership rows for one
// album, then prunes memberships for assets no longer present. Dedup is by the
// generic (source, external_id). The whole album is one transaction so a
// mid-import failure rolls back cleanly. Returns new image count + member count.
func upsertAlbumAssets(db *gorm.DB, source string, albumID uint, assets []RemoteAsset) (newCount, memberCount int, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		// Batch-load existing image ids by external id (one query).
		idByExt := make(map[string]uint, len(assets))
		if len(assets) > 0 {
			extIDs := make([]string, 0, len(assets))
			for _, a := range assets {
				extIDs = append(extIDs, a.ExternalID)
			}
			var existing []model.Image
			if e := tx.Where("source = ? AND external_id IN ?", source, extIDs).
				Find(&existing).Error; e != nil {
				return e
			}
			for _, im := range existing {
				idByExt[im.ExternalID] = im.ID
			}
		}

		// Insert assets we don't have yet.
		for _, a := range assets {
			if a.ExternalID == "" {
				continue
			}
			if _, ok := idByExt[a.ExternalID]; ok {
				continue
			}
			orientation := a.Orientation
			if orientation == "" {
				orientation = determineOrientation(a.Width, a.Height, "")
			}
			img := model.Image{
				Source:       source,
				ExternalID:   a.ExternalID,
				FilePath:     a.FilePath,
				Caption:      a.Caption,
				Width:        a.Width,
				Height:       a.Height,
				Orientation:  orientation,
				ThumbnailKey: a.ThumbnailKey,
				PhotoTakenAt: a.PhotoTakenAt,
				CreatedAt:    time.Now(),
				Status:       "pending",
			}
			if e := tx.Create(&img).Error; e != nil {
				return e
			}
			idByExt[a.ExternalID] = img.ID
			newCount++
		}

		// Existing memberships for this album (one query) → only create missing.
		var memIDs []uint
		tx.Model(&model.ImageAlbumMembership{}).Where("album_id = ?", albumID).Pluck("image_id", &memIDs)
		hasMem := make(map[uint]bool, len(memIDs))
		for _, id := range memIDs {
			hasMem[id] = true
		}

		seen := make([]uint, 0, len(assets))
		var newMems []model.ImageAlbumMembership
		for _, a := range assets {
			imgID := idByExt[a.ExternalID]
			if imgID == 0 {
				continue
			}
			seen = append(seen, imgID)
			memberCount++
			if !hasMem[imgID] {
				hasMem[imgID] = true
				newMems = append(newMems, model.ImageAlbumMembership{ImageID: imgID, AlbumID: albumID})
			}
		}
		if len(newMems) > 0 {
			if e := tx.CreateInBatches(&newMems, 200).Error; e != nil {
				return e
			}
		}

		// Prune memberships for assets no longer in the album.
		prune := tx.Where("album_id = ?", albumID)
		if len(seen) > 0 {
			prune = prune.Where("image_id NOT IN ?", seen)
		}
		return prune.Delete(&model.ImageAlbumMembership{}).Error
	})
	if err != nil {
		return 0, 0, err
	}
	return newCount, memberCount, nil
}

// SetSyncAlbums marks the given albums sync-enabled (creating rows as needed) and
// disables every other REAL album for the source. Virtual albums (e.g. Immich
// all/favorites/memories) are left untouched -- the caller manages those. Shared
// by immich/synology album selection and by topic-source topic management.
func SetSyncAlbums(db *gorm.DB, source string, albums []RemoteAlbum) error {
	return db.Transaction(func(tx *gorm.DB) error {
		wantIDs := make([]string, 0, len(albums))
		for _, a := range albums {
			wantIDs = append(wantIDs, a.ExternalID)
			var existing model.Album
			err := tx.Where("source = ? AND external_id = ?", source, a.ExternalID).First(&existing).Error
			switch {
			case err == gorm.ErrRecordNotFound:
				if e := tx.Create(&model.Album{
					Source:      source,
					ExternalID:  a.ExternalID,
					Name:        a.Name,
					Kind:        model.AlbumKindReal,
					SyncEnabled: true,
				}).Error; e != nil {
					return e
				}
			case err == nil:
				updates := map[string]interface{}{"sync_enabled": true}
				if a.Name != "" {
					updates["name"] = a.Name
				}
				if e := tx.Model(&model.Album{}).Where("id = ?", existing.ID).
					Updates(updates).Error; e != nil {
					return e
				}
			default:
				return err
			}
		}

		// Disable real albums no longer wanted.
		q := tx.Model(&model.Album{}).Where("source = ? AND kind = ?", source, model.AlbumKindReal)
		if len(wantIDs) > 0 {
			q = q.Where("external_id NOT IN ?", wantIDs)
		}
		return q.Update("sync_enabled", false).Error
	})
}
