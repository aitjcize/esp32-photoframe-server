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

// ImportPhotos fetches image assets according to the configured source
// mode (album / all / favorites / memories) and adds them to the DB.
func (s *ImmichService) ImportPhotos() error {
	client, err := s.getClient()
	if err != nil {
		return err
	}

	allAssets, err := s.fetchAssetsForMode(client)
	if err != nil {
		return err
	}

	count := 0
	for _, asset := range allAssets {
		if asset.Type != "IMAGE" {
			continue
		}

		// Skip RAW files — these can't be served via Immich's preview/thumbnail API
		ext := strings.ToLower(filepath.Ext(asset.OriginalFileName))
		switch ext {
		case ".dng", ".cr2", ".cr3", ".nef", ".arw", ".raf", ".orf", ".rw2":
			continue
		}

		// Deduplicate by immich_asset_id
		var existing model.Image
		result := s.db.Where("immich_asset_id = ? AND source = ?", asset.ID, model.SourceImmich).First(&existing)
		if result.Error == nil {
			continue
		}

		// Determine display orientation from EXIF dimensions and rotation
		w, h := asset.ExifInfo.ExifImageWidth, asset.ExifInfo.ExifImageHeight
		orientation := determineOrientation(w, h, asset.ExifInfo.Orientation)

		img := model.Image{
			ImmichAssetID: asset.ID,
			Source:        model.SourceImmich,
			FilePath:      asset.OriginalFileName,
			Width:         w,
			Height:        h,
			Orientation:   orientation,
			CreatedAt:     time.Now(),
			Status:        "pending",
		}

		// Populate PhotoTakenAt from EXIF or asset metadata
		photoDate := parseImmichDate(asset.ExifInfo.DateTimeOriginal)
		if photoDate == nil {
			photoDate = parseImmichDate(asset.LocalDateTime)
		}
		img.PhotoTakenAt = photoDate

		if err := s.db.Create(&img).Error; err != nil {
			log.Printf("Failed to insert immich asset %s: %v", asset.ID, err)
			continue
		}
		count++
	}

	log.Printf("Immich ImportPhotos complete: inserted %d new photos (total assets: %d)", count, len(allAssets))
	return nil
}

// fetchAssetsForMode dispatches to the right Immich client method for the
// configured source mode. Returns the raw asset list — caller filters out
// videos / RAW / duplicates and persists into the local DB.
func (s *ImmichService) fetchAssetsForMode(client *immich.Client) ([]immich.Asset, error) {
	switch s.immichSourceMode() {
	case ImmichModeAlbum:
		albumID, _ := s.settings.Get("immich_album_id")
		if albumID == "" {
			return nil, errors.New("please select an album to sync")
		}
		return client.GetAlbumAssets(albumID)
	case ImmichModeAll:
		return client.SearchAssets(immich.SearchMetadataRequest{})
	case ImmichModeFavorites:
		t := true
		return client.SearchAssets(immich.SearchMetadataRequest{IsFavorite: &t})
	case ImmichModeMemories:
		return client.GetMemoryAssets(s.immichMemoryMode() == ImmichMemoryModeLatest)
	default:
		return nil, fmt.Errorf("unknown immich source mode: %q", s.immichSourceMode())
	}
}

// ClearPhotos deletes all Immich photos from the database
func (s *ImmichService) ClearPhotos() error {
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
