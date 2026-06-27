package service

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/synology"
	"gorm.io/gorm"
)

type SynologyService struct {
	db       *gorm.DB
	settings *SettingsService
	client   *synology.Client
	mu       sync.Mutex
	autoSync *AutoSyncScheduler
}

func NewSynologyService(db *gorm.DB, settings *SettingsService) *SynologyService {
	svc := &SynologyService{
		db:       db,
		settings: settings,
	}
	svc.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:     "Synology",
		Settings: settings,
		IsRelevantKey: func(key string) bool {
			switch key {
			case "synology_auto_sync_enabled", "synology_auto_sync_interval_minutes", "synology_album_id", "synology_url", "synology_account", "synology_password":
				return true
			default:
				return false
			}
		},
		IsConfigured: svc.isAutoSyncConfigured,
		GetConfig:    svc.getAutoSyncConfig,
		RunSync:      svc.clearAndResyncInternal,
	})
	return svc
}

// StartAutoSync starts background synchronization for Synology photos.
func (s *SynologyService) StartAutoSync() {
	s.autoSync.Start()
}

func (s *SynologyService) isAutoSyncConfigured() bool {
	baseURL, _ := s.settings.Get("synology_url")
	account, _ := s.settings.Get("synology_account")
	password, _ := s.settings.Get("synology_password")
	albumID, _ := s.settings.Get("synology_album_id")
	return baseURL != "" && account != "" && password != "" && albumID != ""
}

func (s *SynologyService) getAutoSyncConfig() (bool, time.Duration) {
	return parseAutoSyncConfig(s.settings,
		"synology_auto_sync_enabled", "synology_auto_sync_interval_minutes")
}

// ensureClient initializes and logs in the client if needed
func (s *SynologyService) ensureClient(otpCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseURL, _ := s.settings.Get("synology_url")
	account, _ := s.settings.Get("synology_account")
	password, _ := s.settings.Get("synology_password")
	savedSID, _ := s.settings.Get("synology_sid")
	savedDID, _ := s.settings.Get("synology_did")
	savedToken, _ := s.settings.Get("synology_token")
	skipCertStr, _ := s.settings.Get("synology_skip_cert")
	insecure := skipCertStr == "true"

	if baseURL == "" || account == "" || password == "" {
		return errors.New("synology credentials not configured")
	}

	var parsedURL *url.URL

	// If client exists and has same config, check connectivity or session
	if s.client == nil || s.client.BaseURL != baseURL || s.client.Account != account {
		c, err := synology.NewClient(baseURL, account, password, insecure)
		if err != nil {
			return err
		}
		s.client = c
		// Restore SID, DID and SynoToken if available
		if savedSID != "" {
			s.client.SID = savedSID
			s.client.DID = savedDID
			s.client.SynoToken = savedToken

			// Set cookies in the jar
			parsedURL, err = url.Parse(baseURL)
			if err == nil && s.client.Jar() != nil {
				cookies := []*http.Cookie{
					{Name: "id", Value: savedSID, Path: "/"},
				}
				if savedDID != "" {
					cookies = append(cookies, &http.Cookie{Name: "did", Value: savedDID, Path: "/"})
				}
				s.client.Jar().SetCookies(parsedURL, cookies)
			}
		}
	}

	// Login if no SID or if explicitly requested (otpCode provided implies re-login attempt?)
	if s.client.SID == "" || otpCode != "" {
		if err := s.client.Login(otpCode); err != nil {
			return err
		}
		// Save SID and DID
		s.settings.Set("synology_sid", s.client.SID)
		s.settings.Set("synology_did", s.client.DID)
	}

	// Capture SynoToken from cookie jar or direct client field
	_ = s.settings.Set("synology_token", s.client.SynoToken)

	return nil
}

func (s *SynologyService) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		_ = s.client.Logout()
	}
	s.settings.Set("synology_token", "")
	s.settings.Set("synology_did", "")
	return s.settings.Set("synology_sid", "")
}

func (s *SynologyService) TestConnection(otpCode string) error {
	// Force login to test
	s.mu.Lock()
	// reset client to force reload settings (but ensureClient handles it)
	s.client = nil
	s.mu.Unlock()

	return s.ensureClient(otpCode)
}

func (s *SynologyService) GetPhoto(id int, cacheKeyStr, size string) ([]byte, error) {
	if err := s.ensureClient(""); err != nil {
		return nil, err
	}

	// 1. Find photo in DB to get stored cache key
	var img model.Image
	if err := s.db.Where("synology_photo_id = ? AND source = ?", id, model.SourceSynologyPhotos).First(&img).Error; err != nil {
		// Fallback if not found in DB
		data, getErr := s.client.GetPhoto(id, cacheKeyStr, size, 0, s.client.SynoToken)
		if s.isAuthExpired(getErr) {
			if reErr := s.relogin(); reErr != nil {
				return nil, reErr
			}
			return s.client.GetPhoto(id, cacheKeyStr, size, 0, s.client.SynoToken)
		}
		return data, getErr
	}

	// 2. Resolve the album for this photo (membership first, then the legacy
	// global setting) so multi-album photos are fetched with the right album.
	albumID := s.albumIDForImage(img.ID)

	data, err := s.client.GetPhoto(id, img.ThumbnailKey, size, albumID, s.client.SynoToken)
	if s.isAuthExpired(err) {
		if reErr := s.relogin(); reErr != nil {
			return nil, reErr
		}
		return s.client.GetPhoto(id, img.ThumbnailKey, size, albumID, s.client.SynoToken)
	}
	return data, err
}

// albumIDForImage returns a Synology album external id (int) the image belongs
// to via membership, falling back to the legacy global setting, else 0.
func (s *SynologyService) albumIDForImage(imageID uint) int {
	var ext string
	s.db.Model(&model.ImageAlbumMembership{}).
		Joins("JOIN albums ON albums.id = image_album_memberships.album_id").
		Where("image_album_memberships.image_id = ? AND albums.source = ?", imageID, model.SourceSynologyPhotos).
		Limit(1).Pluck("albums.external_id", &ext)
	if ext != "" {
		if n, err := strconv.Atoi(ext); err == nil {
			return n
		}
	}
	if g, _ := s.settings.Get("synology_album_id"); g != "" {
		if n, err := strconv.Atoi(g); err == nil {
			return n
		}
	}
	return 0
}

// relogin clears the expired session and attempts to re-authenticate.
// The saved DID (device token) allows bypassing 2FA on trusted devices.
func (s *SynologyService) relogin() error {
	s.mu.Lock()
	s.client.SID = ""
	s.mu.Unlock()
	s.settings.Set("synology_sid", "")
	log.Printf("Synology session expired, attempting re-login with saved device token")
	return s.ensureClient("")
}

func (s *SynologyService) isAuthExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "code: 119")
}

func (s *SynologyService) ListAlbums() ([]synology.Album, error) {
	if err := s.ensureClient(""); err != nil {
		return nil, err
	}

	albums, err := s.client.ListAlbums(0, 100)
	if s.isAuthExpired(err) {
		if reErr := s.relogin(); reErr != nil {
			return nil, errors.New("authentication expired and re-login failed: " + reErr.Error())
		}
		albums, err = s.client.ListAlbums(0, 100)
	}
	if err != nil {
		return nil, err
	}

	// Cache the albums list
	albumsJSON, _ := json.Marshal(albums)
	s.settings.Set("synology_albums_cache", string(albumsJSON))

	return albums, nil
}

// ImportPhotos syncs every sync-enabled Synology album into the DB, upserting
// image rows (deduped by synology_photo_id) plus (asset, album) memberships,
// pruning stale memberships and orphaned image rows.
func (s *SynologyService) ImportPhotos() error {
	if err := s.ensureClient(""); err != nil {
		return err
	}

	s.ensureGlobalAlbumSeed()

	var albums []model.Album
	if err := s.db.Where("source = ? AND sync_enabled = ?", model.SourceSynologyPhotos, true).
		Find(&albums).Error; err != nil {
		return err
	}
	if len(albums) == 0 {
		log.Println("Synology ImportPhotos: no albums enabled for sync")
		return nil
	}

	totalNew := 0
	for _, album := range albums {
		albumID, err := strconv.Atoi(album.ExternalID)
		if err != nil {
			log.Printf("Synology: invalid album external id %q", album.ExternalID)
			continue
		}
		newCount, _, aerr := s.importAlbumAssets(album, albumID)
		if aerr != nil {
			log.Printf("Synology: import album %q (%s) failed: %v", album.Name, album.ExternalID, aerr)
			continue // leave the album's prior state untouched
		}
		totalNew += newCount
		if err := s.db.Model(&model.Album{}).Where("id = ?", album.ID).
			Updates(map[string]interface{}{"updated_at": time.Now()}).Error; err != nil {
			log.Printf("[synology] update album %d (%q) updated_at: %v", album.ID, album.Name, err)
		}
	}

	s.gcOrphanImages()
	log.Printf("Synology ImportPhotos complete: %d new photos across %d album(s)", totalNew, len(albums))
	return nil
}

// importAlbumAssets pages through one Synology album, upserting image + (asset,
// album) membership rows, then prunes memberships for photos removed from it.
func (s *SynologyService) importAlbumAssets(album model.Album, albumID int) (newCount, memberCount int, err error) {
	// Phase 1: page through the album over the network. Done BEFORE opening a DB
	// transaction so we never hold a tx open across HTTP calls.
	var photos []synology.Item
	offset, limit := 0, 500
	for offset < 5000 {
		batch, e := s.client.ListPhotos(offset, limit, albumID)
		if s.isAuthExpired(e) {
			if reErr := s.relogin(); reErr != nil {
				return 0, 0, reErr
			}
			batch, e = s.client.ListPhotos(offset, limit, albumID)
		}
		if e != nil {
			return 0, 0, e
		}
		if len(batch) == 0 {
			break
		}
		photos = append(photos, batch...)
		if len(batch) < limit {
			break
		}
		offset += limit
	}

	// Phase 2: all DB writes for the album in one transaction.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Batch-load existing rows by synology_photo_id (one query).
		existingByPhoto := make(map[int]model.Image, len(photos))
		if len(photos) > 0 {
			photoIDs := make([]int, len(photos))
			for i, p := range photos {
				photoIDs[i] = p.ID
			}
			var existing []model.Image
			if e := tx.Where("source = ? AND synology_photo_id IN ?", model.SourceSynologyPhotos, photoIDs).
				Find(&existing).Error; e != nil {
				return e
			}
			for _, im := range existing {
				existingByPhoto[im.SynologyPhotoID] = im
			}
		}

		idForPhoto := make(map[int]uint, len(photos))
		for pid, im := range existingByPhoto {
			idForPhoto[pid] = im.ID
		}

		for _, p := range photos {
			if existImg, ok := existingByPhoto[p.ID]; ok {
				// Backfill a missing thumbnail key on an existing row.
				if existImg.ThumbnailKey == "" && p.Additional.Thumbnail.M != "" {
					if e := tx.Model(&model.Image{}).Where("id = ?", existImg.ID).
						Update("thumbnail_key", p.Additional.Thumbnail.M).Error; e != nil {
						return e
					}
				}
				continue
			}
			pw, ph := p.Additional.Resolution.Width, p.Additional.Resolution.Height
			img := model.Image{
				SynologyPhotoID: p.ID,
				Source:          model.SourceSynologyPhotos,
				FilePath:        p.Filename,
				ThumbnailKey:    p.Additional.Thumbnail.M,
				Width:           pw,
				Height:          ph,
				Orientation:     determineOrientation(pw, ph, ""),
				CreatedAt:       time.Now(),
				Status:          "pending",
			}
			if p.Additional.Thumbnail.XL != "" {
				img.ThumbnailKey = p.Additional.Thumbnail.XL
			}
			if p.Time > 0 {
				t := time.Unix(p.Time, 0)
				img.PhotoTakenAt = &t
			}
			if e := tx.Create(&img).Error; e != nil {
				return e
			}
			idForPhoto[p.ID] = img.ID
			newCount++
		}

		// Existing memberships for this album → only create the missing ones.
		var memIDs []uint
		tx.Model(&model.ImageAlbumMembership{}).Where("album_id = ?", album.ID).Pluck("image_id", &memIDs)
		hasMem := make(map[uint]bool, len(memIDs))
		for _, id := range memIDs {
			hasMem[id] = true
		}

		seen := make([]uint, 0, len(photos))
		var newMems []model.ImageAlbumMembership
		for _, p := range photos {
			imgID := idForPhoto[p.ID]
			if imgID == 0 {
				continue
			}
			seen = append(seen, imgID)
			memberCount++
			if !hasMem[imgID] {
				hasMem[imgID] = true
				newMems = append(newMems, model.ImageAlbumMembership{ImageID: imgID, AlbumID: album.ID})
			}
		}
		if len(newMems) > 0 {
			if e := tx.CreateInBatches(&newMems, 200).Error; e != nil {
				return e
			}
		}

		// Prune memberships for photos no longer in the album.
		prune := tx.Where("album_id = ?", album.ID)
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

// gcOrphanImages removes Synology image rows no longer a member of any album.
func (s *SynologyService) gcOrphanImages() {
	gcOrphanImagesForSource(s.db, model.SourceSynologyPhotos)
}

// ensureGlobalAlbumSeed materializes the legacy synology_album_id setting into
// a sync-enabled albums row once, for back-compat. Idempotent.
func (s *SynologyService) ensureGlobalAlbumSeed() {
	var count int64
	s.db.Model(&model.Album{}).Where("source = ?", model.SourceSynologyPhotos).Count(&count)
	if count > 0 {
		return
	}
	ext, _ := s.settings.Get("synology_album_id")
	if ext == "" {
		return
	}
	name := ext
	if albums, err := s.ListAlbums(); err == nil {
		for _, a := range albums {
			if strconv.Itoa(a.ID) == ext {
				name = a.Name
				break
			}
		}
	}
	album := model.Album{
		Source: model.SourceSynologyPhotos, ExternalID: ext,
		Kind: model.AlbumKindReal, Name: name, SyncEnabled: true, UpdatedAt: time.Now(),
	}
	if err := s.db.Create(&album).Error; err != nil {
		log.Printf("Synology: failed to seed global album row: %v", err)
	}
}

// SetSyncAlbums defines which Synology albums (by external id) to sync: enables
// those album rows, disables the rest, then clear+resyncs.
func (s *SynologyService) SetSyncAlbums(realIDs []string) error {
	nameByID := map[string]string{}
	if albums, err := s.ListAlbums(); err == nil {
		for _, a := range albums {
			nameByID[strconv.Itoa(a.ID)] = a.Name
		}
	}

	desired := map[string]bool{}
	for _, id := range realIDs {
		if id == "" {
			continue
		}
		desired[id] = true
		name := nameByID[id]
		if name == "" {
			name = id
		}
		var existing model.Album
		if err := s.db.Where("source = ? AND external_id = ?", model.SourceSynologyPhotos, id).
			First(&existing).Error; err != nil {
			if err := s.db.Create(&model.Album{
				Source: model.SourceSynologyPhotos, ExternalID: id,
				Kind: model.AlbumKindReal, Name: name, SyncEnabled: true, UpdatedAt: time.Now(),
			}).Error; err != nil {
				log.Printf("[synology] create album (external_id=%s, name=%q): %v", id, name, err)
			}
		} else {
			if err := s.db.Model(&model.Album{}).Where("id = ?", existing.ID).
				Updates(map[string]interface{}{"sync_enabled": true, "name": name}).Error; err != nil {
				log.Printf("[synology] update album %d (external_id=%s): %v", existing.ID, id, err)
			}
		}
	}
	var rows []model.Album
	s.db.Where("source = ?", model.SourceSynologyPhotos).Find(&rows)
	for _, a := range rows {
		if !desired[a.ExternalID] && a.SyncEnabled {
			if err := s.db.Model(&model.Album{}).Where("id = ?", a.ID).Update("sync_enabled", false).Error; err != nil {
				log.Printf("[synology] disable album %d (external_id=%s): %v", a.ID, a.ExternalID, err)
			}
		}
	}
	// Persist the selection only — the import is triggered explicitly (manual
	// Sync) or by the auto-sync scheduler, not on every album toggle.
	return nil
}

// ClearPhotos deletes all Synology photos and their album memberships.
func (s *SynologyService) ClearPhotos() error {
	return clearSourcePhotos(s.db, model.SourceSynologyPhotos)
}

// ClearAndResync deletes all Synology photos and re-imports them
func (s *SynologyService) ClearAndResync() error {
	// Run the clear+resync in the background so the manual Sync request returns
	// promptly; the client polls sync-status to show progress.
	s.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether a Synology sync is currently running.
func (s *SynologyService) IsSyncing() bool {
	return s.autoSync.IsRunning()
}

func (s *SynologyService) clearAndResyncInternal() error {
	if err := s.ClearPhotos(); err != nil {
		return err
	}
	return s.ImportPhotos()
}

// GetPhotoCount returns the number of Synology photos in the database
func (s *SynologyService) GetPhotoCount() (int64, error) {
	return sourcePhotoCount(s.db, model.SourceSynologyPhotos)
}

// DownloadPhoto fetches the large thumbnail by ID (avoiding full download/EXIF issues)
func (s *SynologyService) DownloadPhoto(id int) ([]byte, error) {
	// Re-use GetPhoto logic which handles DB lookup, cache keys, and space
	return s.GetPhoto(id, "", "large")
}
