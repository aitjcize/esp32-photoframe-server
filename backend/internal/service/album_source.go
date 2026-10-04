package service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"gorm.io/gorm"
)

// RemoteAsset is the source-agnostic upsert payload for one asset. Each source
// maps its API response to this; it is the ONLY per-source difference the shared
// album-sync engine sees.
type RemoteAsset struct {
	ExternalID string // stable source id (immich UUID, synology id, unsplash/pexels id)
	// FilePath is how this source's downloader later fetches the bytes: a local
	// path for disk sources (immich/synology fetch by id instead), or the image
	// URL for URL-based sources (unsplash/pexels).
	FilePath     string
	Caption      string // optional (e.g. photographer credit)
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
	Passphrase string // optional (synology shared-album access token)
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
// A failing album doesn't stop the loop (its prior state is left untouched), but
// the failures are aggregated into the returned error so callers can surface
// them instead of reporting a silent "0 new photos" success — see issue #44.
// Generalizes what immich.go/synology.go's ImportPhotos each hand-rolled.
func SyncAlbumSource(db *gorm.DB, src AlbumSource) (int, error) {
	source := src.Source()

	var albums []model.Album
	if err := db.Where("source = ? AND sync_enabled = ?", source, true).Find(&albums).Error; err != nil {
		return 0, err
	}

	// Albums unchecked since the last sync keep their membership rows, which
	// would shield their images from the orphan GC below. Drop them first so
	// a resync removes what the user deselected.
	pruneDisabledAlbumMemberships(db, source)

	if len(albums) == 0 {
		gcOrphanImagesForSource(db, source)
		log.Printf("%s sync: no albums enabled for sync", source)
		return 0, nil
	}

	// Best-effort metadata refresh for real albums: the name for display, and
	// the Synology share passphrase, which follows what the NAS reports now.
	// DSM can rotate the passphrase of a shared album, and a stale one would
	// fail every future fetch of that album with no way back. An owned album
	// may still hold the passphrase of a link it was shared out by (stored
	// before ListAlbums dropped those): the account reaches its own albums by
	// id, and DSM refuses album_id and passphrase together (error 120), so
	// that one has to go.
	remoteByExternalID := map[string]RemoteAlbum{}
	if list, e := src.ListRemoteAlbums(); e == nil {
		for _, a := range list {
			remoteByExternalID[a.ExternalID] = a
		}
	}

	total := 0
	var failures []string
	for _, album := range albums {
		// Only an album the listing returned says anything about its
		// passphrase. A listing that failed (NAS unreachable) or no longer
		// carries the album leaves the stored one alone.
		remote, listed := remoteByExternalID[album.ExternalID]
		if listed && album.Kind == model.AlbumKindReal &&
			remote.Passphrase != album.SharePassphrase {
			album.SharePassphrase = remote.Passphrase
			if e := db.Model(&model.Album{}).Where("id = ?", album.ID).
				Update("share_passphrase", remote.Passphrase).Error; e != nil {
				log.Printf("%s: update album %d passphrase: %v", source, album.ID, e)
			}
		}

		assets, err := src.FetchAlbumAssets(album)
		if err != nil {
			log.Printf("%s: fetch album %q (%s) failed: %v", source, album.Name, album.ExternalID, err)
			failures = append(failures, fmt.Sprintf("%s: %v", album.Name, err))
			continue // leave the album's prior state + count untouched
		}
		newCount, _, err := upsertAlbumAssets(db, source, album.ID, assets)
		if err != nil {
			log.Printf("%s: import album %q (%s) failed: %v", source, album.Name, album.ExternalID, err)
			failures = append(failures, fmt.Sprintf("%s: %v", album.Name, err))
			continue
		}
		total += newCount

		updates := map[string]interface{}{"updated_at": time.Now()}
		if album.Kind == model.AlbumKindReal && remote.Name != "" {
			updates["name"] = remote.Name
		}
		if err := db.Model(&model.Album{}).Where("id = ?", album.ID).Updates(updates).Error; err != nil {
			log.Printf("%s: update album %d (%q): %v", source, album.ID, album.Name, err)
		}
	}

	gcOrphanImagesForSource(db, source)
	log.Printf("%s sync complete: %d new photos across %d album(s)", source, total, len(albums))
	if len(failures) > 0 {
		return total, fmt.Errorf("%d of %d album(s) failed to sync — %s",
			len(failures), len(albums), strings.Join(failures, "; "))
	}
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

// countAlbums returns how many album rows exist for a source and how many of
// them are sync-enabled. Callers use the pair to tell "the album picker has
// never been used here" (total == 0, fall back to the legacy single-album
// settings) from "the picker is in use and everything is deselected"
// (total > 0, enabled == 0).
func countAlbums(db *gorm.DB, source string) (total, enabled int64) {
	if err := db.Model(&model.Album{}).Where("source = ?", source).
		Count(&total).Error; err != nil {
		log.Printf("%s: count albums: %v", source, err)
		return 0, 0
	}
	if err := db.Model(&model.Album{}).Where("source = ? AND sync_enabled = ?", source, true).
		Count(&enabled).Error; err != nil {
		log.Printf("%s: count sync-enabled albums: %v", source, err)
		return total, 0
	}
	return total, enabled
}

// SetSyncAlbums marks the given albums sync-enabled (creating rows as needed) and
// disables every other REAL album for the source. Virtual albums (e.g. Immich
// all/favorites/memories) are left untouched -- the caller manages those. Shared
// by immich/synology album selection and by topic-source topic management.
// Images of the deselected albums are removed immediately (metadata only; a
// re-check plus resync re-imports them), matching topic-source behavior.
func SetSyncAlbums(db *gorm.DB, source string, albums []RemoteAlbum) error {
	err := db.Transaction(func(tx *gorm.DB) error {
		wantIDs := make([]string, 0, len(albums))
		for _, a := range albums {
			wantIDs = append(wantIDs, a.ExternalID)
			var existing model.Album
			err := tx.Where("source = ? AND external_id = ?", source, a.ExternalID).First(&existing).Error
			switch {
			case err == gorm.ErrRecordNotFound:
				if e := tx.Create(&model.Album{
					Source:          source,
					ExternalID:      a.ExternalID,
					Name:            a.Name,
					Kind:            model.AlbumKindReal,
					SyncEnabled:     true,
					SharePassphrase: a.Passphrase,
				}).Error; e != nil {
					return e
				}
			case err == nil:
				updates := map[string]interface{}{"sync_enabled": true}
				if a.Name != "" {
					updates["name"] = a.Name
				}
				// Only overwrite a stored passphrase with a real one: sources
				// without passphrases pass "", and so does Synology when the
				// NAS was unreachable while the user saved the selection.
				if a.Passphrase != "" {
					updates["share_passphrase"] = a.Passphrase
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
	if err != nil {
		return err
	}
	// Drop the deselected albums' images right away instead of waiting for the
	// next sync, so the gallery reflects the new selection immediately.
	pruneDisabledAlbumMemberships(db, source)
	gcOrphanImagesForSource(db, source)
	return nil
}
