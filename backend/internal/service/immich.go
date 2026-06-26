package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/immich"
	"gorm.io/gorm"
)

type ImmichService struct {
	db       *gorm.DB
	settings *SettingsService
	client   *immich.Client
	mu       sync.Mutex
	autoSync *AutoSyncScheduler
}

func NewImmichService(db *gorm.DB, settings *SettingsService) *ImmichService {
	svc := &ImmichService{db: db, settings: settings}
	svc.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:     "Immich",
		Settings: settings,
		IsRelevantKey: func(key string) bool {
			switch key {
			case "immich_auto_sync_enabled", "immich_auto_sync_interval_minutes",
				"immich_source_mode", "immich_memory_mode", "immich_album_id",
				"immich_url", "immich_api_key":
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

// StartAutoSync starts a background loop that periodically syncs Immich photos
// when the corresponding settings are enabled.
func (s *ImmichService) StartAutoSync() {
	s.autoSync.Start()
}

// Immich source modes — what the sync pulls from. See issue #32.
const (
	ImmichModeAlbum     = "album"     // photos from one configured album (default)
	ImmichModeAll       = "all"       // entire library
	ImmichModeFavorites = "favorites" // only assets marked as Favorite
	ImmichModeMemories  = "memories"  // on-this-day across years
)

// Immich memory modes — how the "memories" source picks lanes. See issue #32.
const (
	ImmichMemoryModeAll    = "all"    // flatten all "on this day" lanes across years (default)
	ImmichMemoryModeLatest = "latest" // only the most recent year's lane
)

// immichSourceMode returns the configured sync mode, defaulting to album.
func (s *ImmichService) immichSourceMode() string {
	mode, _ := s.settings.Get("immich_source_mode")
	switch mode {
	case ImmichModeAll, ImmichModeFavorites, ImmichModeMemories:
		return mode
	default:
		return ImmichModeAlbum
	}
}

// immichMemoryMode returns the configured memories lane selection, defaulting
// to all (every year flattened into one pool).
func (s *ImmichService) immichMemoryMode() string {
	if mode, _ := s.settings.Get("immich_memory_mode"); mode == ImmichMemoryModeLatest {
		return ImmichMemoryModeLatest
	}
	return ImmichMemoryModeAll
}

func (s *ImmichService) isAutoSyncConfigured() bool {
	baseURL, _ := s.settings.Get("immich_url")
	apiKey, _ := s.settings.Get("immich_api_key")
	if baseURL == "" || apiKey == "" {
		return false
	}
	// Album mode is the only one that needs an album picked.
	if s.immichSourceMode() == ImmichModeAlbum {
		albumID, _ := s.settings.Get("immich_album_id")
		return albumID != ""
	}
	return true
}

func (s *ImmichService) getAutoSyncConfig() (bool, time.Duration) {
	enabledStr, _ := s.settings.Get("immich_auto_sync_enabled")
	enabled := strings.EqualFold(enabledStr, "true")

	minutes := 60
	if intervalStr, err := s.settings.Get("immich_auto_sync_interval_minutes"); err == nil {
		if parsed, parseErr := strconv.Atoi(intervalStr); parseErr == nil && parsed > 0 {
			minutes = parsed
		}
	}

	return enabled, time.Duration(minutes) * time.Minute
}

// getClient returns the current client, initializing from stored settings if needed.
// The returned client pointer is safe to use even if s.client is later replaced.
func (s *ImmichService) getClient() (*immich.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseURL, _ := s.settings.Get("immich_url")
	apiKey, _ := s.settings.Get("immich_api_key")

	if baseURL == "" || apiKey == "" {
		return nil, errors.New("immich credentials not configured")
	}

	if s.client == nil || s.client.BaseURL != baseURL || s.client.APIKey != apiKey {
		s.client = immich.NewClient(baseURL, apiKey)
	}
	return s.client, nil
}

// TestConnection creates a fresh client from settings and verifies connectivity
func (s *ImmichService) TestConnection() error {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	client, err := s.getClient()
	if err != nil {
		return err
	}
	return client.TestConnection()
}

// ListAlbums returns all albums accessible with the configured API key
func (s *ImmichService) ListAlbums() ([]immich.Album, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.ListAlbums()
}

// SetSyncAlbums defines the set of Immich albums to sync: the given real album
// external IDs plus the enabled virtual modes (favorites/all/memories). Albums
// in the set are upserted with sync_enabled=true; all other Immich album rows
// are disabled. A full clear+resync then rebuilds the local pool + memberships.
func (s *ImmichService) SetSyncAlbums(realIDs []string, favorites, all, memories bool) error {
	// Resolve names for real albums (best-effort).
	nameByID := map[string]string{}
	if list, err := s.ListAlbums(); err == nil {
		for _, a := range list {
			nameByID[a.ID] = a.AlbumName
		}
	}

	desired := map[string]model.Album{}
	for _, id := range realIDs {
		if id == "" {
			continue
		}
		name := nameByID[id]
		if name == "" {
			name = id
		}
		desired[id] = model.Album{
			Source: model.SourceImmich, ExternalID: id,
			Kind: model.AlbumKindReal, Name: name, SyncEnabled: true,
		}
	}
	addVirtual := func(on bool, ext, name string) {
		if on {
			desired[ext] = model.Album{
				Source: model.SourceImmich, ExternalID: ext,
				Kind: model.AlbumKindVirtual, Name: name, SyncEnabled: true,
			}
		}
	}
	addVirtual(all, model.ImmichVirtualAll, "All Photos")
	addVirtual(favorites, model.ImmichVirtualFavorites, "Favorites")
	addVirtual(memories, model.ImmichVirtualMemories, "Memories")

	// Upsert desired albums (enabled); disable everything else.
	for ext, a := range desired {
		var existing model.Album
		if err := s.db.Where("source = ? AND external_id = ?", model.SourceImmich, ext).
			First(&existing).Error; err != nil {
			a.UpdatedAt = time.Now()
			s.db.Create(&a)
		} else {
			s.db.Model(&model.Album{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"sync_enabled": true, "name": a.Name, "kind": a.Kind,
			})
		}
	}
	var rows []model.Album
	s.db.Where("source = ?", model.SourceImmich).Find(&rows)
	for _, a := range rows {
		if _, ok := desired[a.ExternalID]; !ok && a.SyncEnabled {
			s.db.Model(&model.Album{}).Where("id = ?", a.ID).Update("sync_enabled", false)
		}
	}

	return s.ClearAndResync()
}

// ImportPhotos syncs every sync-enabled Immich album into the local DB:
// upserting one images row per asset (deduped by immich_asset_id) plus an
// image_album_memberships row per (asset, album). Stale memberships and
// orphaned image rows are pruned so the local pool tracks Immich.
func (s *ImmichService) ImportPhotos() error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	// Materialize the legacy single-album / mode config into an albums row so
	// existing setups keep syncing after the multi-album migration.
	s.ensureGlobalAlbumSeed(client)

	var albums []model.Album
	if err := s.db.Where("source = ? AND sync_enabled = ?", model.SourceImmich, true).
		Find(&albums).Error; err != nil {
		return err
	}
	if len(albums) == 0 {
		log.Println("Immich ImportPhotos: no albums enabled for sync")
		return nil
	}

	// Best-effort name refresh for real albums.
	nameByExternalID := map[string]string{}
	if list, e := client.ListAlbums(); e == nil {
		for _, a := range list {
			nameByExternalID[a.ID] = a.AlbumName
		}
	}

	totalNew := 0
	for _, album := range albums {
		assets, err := s.fetchAssetsForAlbum(client, album)
		if err != nil {
			log.Printf("Immich: failed to fetch assets for album %q (%s): %v",
				album.Name, album.ExternalID, err)
			continue
		}
		newCount, memberCount := s.importAlbumAssets(album, assets)
		totalNew += newCount

		updates := map[string]interface{}{"asset_count": memberCount, "updated_at": time.Now()}
		if album.Kind == model.AlbumKindReal {
			if n := nameByExternalID[album.ExternalID]; n != "" {
				updates["name"] = n
			}
		}
		s.db.Model(&model.Album{}).Where("id = ?", album.ID).Updates(updates)
	}

	s.gcOrphanImages()
	log.Printf("Immich ImportPhotos complete: %d new photos across %d album(s)", totalNew, len(albums))
	return nil
}

// importAlbumAssets upserts image rows and (asset, album) membership rows for
// one album, then prunes memberships for assets no longer in the album.
// Returns the count of newly-created image rows and the album's member count.
func (s *ImmichService) importAlbumAssets(album model.Album, assets []immich.Asset) (newCount, memberCount int) {
	seen := make([]uint, 0, len(assets))
	for _, asset := range assets {
		if asset.Type != "IMAGE" {
			continue
		}
		// Skip RAW — can't be served via Immich's preview/thumbnail API.
		switch strings.ToLower(filepath.Ext(asset.OriginalFileName)) {
		case ".dng", ".cr2", ".cr3", ".nef", ".arw", ".raf", ".orf", ".rw2":
			continue
		}

		var img model.Image
		res := s.db.Where("immich_asset_id = ? AND source = ?", asset.ID, model.SourceImmich).First(&img)
		if res.Error != nil {
			w, h := asset.ExifInfo.ExifImageWidth, asset.ExifInfo.ExifImageHeight
			img = model.Image{
				ImmichAssetID: asset.ID,
				Source:        model.SourceImmich,
				FilePath:      asset.OriginalFileName,
				Width:         w,
				Height:        h,
				Orientation:   determineOrientation(w, h, asset.ExifInfo.Orientation),
				CreatedAt:     time.Now(),
				Status:        "pending",
			}
			photoDate := parseImmichDate(asset.ExifInfo.DateTimeOriginal)
			if photoDate == nil {
				photoDate = parseImmichDate(asset.LocalDateTime)
			}
			img.PhotoTakenAt = photoDate
			if err := s.db.Create(&img).Error; err != nil {
				log.Printf("Failed to insert immich asset %s: %v", asset.ID, err)
				continue
			}
			newCount++
		}

		membership := model.ImageAlbumMembership{ImageID: img.ID, AlbumID: album.ID}
		s.db.FirstOrCreate(&membership, membership)
		seen = append(seen, img.ID)
		memberCount++
	}

	// Prune memberships for assets removed from this album.
	prune := s.db.Where("album_id = ?", album.ID)
	if len(seen) > 0 {
		prune = prune.Where("image_id NOT IN ?", seen)
	}
	prune.Delete(&model.ImageAlbumMembership{})
	return newCount, memberCount
}

// gcOrphanImages removes Immich image rows no longer a member of any album
// (their album was disabled, or they were removed upstream).
func (s *ImmichService) gcOrphanImages() {
	sub := s.db.Model(&model.ImageAlbumMembership{}).Select("image_id")
	s.db.Unscoped().
		Where("source = ? AND id NOT IN (?)", model.SourceImmich, sub).
		Delete(&model.Image{})
}

// fetchAssetsForAlbum returns the assets for one album — a real Immich album
// or a virtual mode album (all / favorites / memories).
func (s *ImmichService) fetchAssetsForAlbum(client *immich.Client, album model.Album) ([]immich.Asset, error) {
	if album.Kind == model.AlbumKindVirtual {
		switch album.ExternalID {
		case model.ImmichVirtualAll:
			return client.SearchAssets(immich.SearchMetadataRequest{})
		case model.ImmichVirtualFavorites:
			t := true
			return client.SearchAssets(immich.SearchMetadataRequest{IsFavorite: &t})
		case model.ImmichVirtualMemories:
			return client.GetMemoryAssets(s.immichMemoryMode() == ImmichMemoryModeLatest)
		default:
			return nil, fmt.Errorf("unknown virtual album: %q", album.ExternalID)
		}
	}
	return client.GetAlbumAssets(album.ExternalID)
}

// ensureGlobalAlbumSeed materializes the legacy global immich_source_mode /
// immich_album_id settings into a sync-enabled albums row, once, so existing
// installs keep working after the multi-album migration. Idempotent: skips if
// any Immich album row already exists.
func (s *ImmichService) ensureGlobalAlbumSeed(client *immich.Client) {
	var count int64
	s.db.Model(&model.Album{}).Where("source = ?", model.SourceImmich).Count(&count)
	if count > 0 {
		return
	}

	var ext, kind, name string
	switch s.immichSourceMode() {
	case ImmichModeAlbum:
		ext, _ = s.settings.Get("immich_album_id")
		if ext == "" {
			return // nothing configured yet
		}
		kind, name = model.AlbumKindReal, ext
		if list, e := client.ListAlbums(); e == nil {
			for _, a := range list {
				if a.ID == ext {
					name = a.AlbumName
					break
				}
			}
		}
	case ImmichModeAll:
		ext, kind, name = model.ImmichVirtualAll, model.AlbumKindVirtual, "All Photos"
	case ImmichModeFavorites:
		ext, kind, name = model.ImmichVirtualFavorites, model.AlbumKindVirtual, "Favorites"
	case ImmichModeMemories:
		ext, kind, name = model.ImmichVirtualMemories, model.AlbumKindVirtual, "Memories"
	default:
		return
	}

	album := model.Album{
		Source:      model.SourceImmich,
		ExternalID:  ext,
		Kind:        kind,
		Name:        name,
		SyncEnabled: true,
		UpdatedAt:   time.Now(),
	}
	if err := s.db.Create(&album).Error; err != nil {
		log.Printf("Immich: failed to seed global album row: %v", err)
	}
}

// ClearPhotos deletes all Immich photos from the database
func (s *ImmichService) ClearPhotos() error {
	// Drop memberships for Immich albums first so they don't dangle.
	var albumIDs []uint
	s.db.Model(&model.Album{}).Where("source = ?", model.SourceImmich).Pluck("id", &albumIDs)
	if len(albumIDs) > 0 {
		s.db.Where("album_id IN ?", albumIDs).Delete(&model.ImageAlbumMembership{})
	}
	if err := s.db.Unscoped().Where("source = ?", model.SourceImmich).Delete(&model.Image{}).Error; err != nil {
		return err
	}
	log.Println("Cleared all Immich photos from database")
	return nil
}

// ClearAndResync deletes all Immich photos and re-imports from the configured album
func (s *ImmichService) ClearAndResync() error {
	return s.autoSync.SyncNow()
}

func (s *ImmichService) clearAndResyncInternal() error {
	if err := s.ClearPhotos(); err != nil {
		return err
	}
	return s.ImportPhotos()
}

// parseImmichDate parses ISO 8601 date strings from the Immich API.
func parseImmichDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return &t
		}
	}
	return nil
}

// GetPhotoCount returns the number of Immich photos in the database
func (s *ImmichService) GetPhotoCount() (int64, error) {
	var count int64
	if err := s.db.Model(&model.Image{}).Where("source = ?", model.SourceImmich).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetPhoto fetches the image bytes for an Immich asset by its UUID.
// size is "thumbnail" (small, for gallery) or "preview" (large, for serving).
func (s *ImmichService) GetPhoto(assetID, size string) ([]byte, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetThumbnail(assetID, size)
}

// DownloadOriginal fetches the full-resolution original image for an asset.
func (s *ImmichService) DownloadOriginal(assetID string) ([]byte, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.DownloadOriginal(assetID)
}

// DownloadPhoto downloads the original full-resolution image and converts it
// to JPEG using ImageMagick (handles HEIC, RAW formats and EXIF auto-orient).
// Falls back to Immich's preview API if original download or conversion fails.
func (s *ImmichService) DownloadPhoto(assetID string) ([]byte, error) {
	data, err := s.DownloadOriginal(assetID)
	if err != nil {
		log.Printf("Immich original download failed for asset %s: %v, falling back to preview", assetID, err)
		return s.downloadPreviewFallback(assetID, err)
	}

	tmpDir, err := os.MkdirTemp("", "immich-convert-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input")
	outputPath := filepath.Join(tmpDir, "output.jpg")

	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}

	// Use ImageMagick to convert any format to JPEG with EXIF auto-orientation
	cmd := exec.Command("magick", inputPath, "-auto-orient", "-quality", "95", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ImageMagick conversion failed for asset %s: %v (output: %s), falling back to preview", assetID, err, string(output))
		return s.downloadPreviewFallback(assetID, err)
	}

	return os.ReadFile(outputPath)
}

// downloadPreviewFallback tries the Immich preview API as a fallback when
// original download or conversion fails.
func (s *ImmichService) downloadPreviewFallback(assetID string, originalErr error) ([]byte, error) {
	previewData, previewErr := s.GetPhoto(assetID, "preview")
	if previewErr != nil {
		return nil, fmt.Errorf("original failed: %w; preview fallback also failed: %v", originalErr, previewErr)
	}
	return previewData, nil
}
