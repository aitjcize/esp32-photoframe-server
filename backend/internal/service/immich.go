package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
}

func NewImmichService(db *gorm.DB, settings *SettingsService) *ImmichService {
	return &ImmichService{db: db, settings: settings}
}

// ensureClient initializes the client from stored settings if needed
func (s *ImmichService) ensureClient() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseURL, _ := s.settings.Get("immich_url")
	apiKey, _ := s.settings.Get("immich_api_key")

	if baseURL == "" || apiKey == "" {
		return errors.New("immich credentials not configured")
	}

	if s.client == nil || s.client.BaseURL != baseURL || s.client.APIKey != apiKey {
		s.client = immich.NewClient(baseURL, apiKey)
	}
	return nil
}

// TestConnection creates a fresh client from settings and verifies connectivity
func (s *ImmichService) TestConnection() error {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	if err := s.ensureClient(); err != nil {
		return err
	}
	return s.client.TestConnection()
}

// ListAlbums returns all albums accessible with the configured API key
func (s *ImmichService) ListAlbums() ([]immich.Album, error) {
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	return s.client.ListAlbums()
}

// ImportPhotos fetches image assets and adds them to the DB.
// ImportPhotos fetches image assets from the configured album and adds them to the DB.
func (s *ImmichService) ImportPhotos() error {
	if err := s.ensureClient(); err != nil {
		return err
	}

	albumID, _ := s.settings.Get("immich_album_id")
	if albumID == "" {
		return errors.New("please select an album to sync")
	}

	allAssets, err := s.client.GetAlbumAssets(albumID)
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

		// Determine orientation from EXIF dimensions
		orientation := "landscape"
		w, h := asset.ExifInfo.ExifImageWidth, asset.ExifInfo.ExifImageHeight
		if h > w && w > 0 {
			orientation = "portrait"
		}

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

		if err := s.db.Create(&img).Error; err != nil {
			log.Printf("Failed to insert immich asset %s: %v", asset.ID, err)
			continue
		}
		count++
	}

	log.Printf("Immich ImportPhotos complete: inserted %d new photos (total assets: %d)", count, len(allAssets))
	return nil
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
	if err := s.ClearPhotos(); err != nil {
		return err
	}
	return s.ImportPhotos()
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
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	return s.client.GetThumbnail(assetID, size)
}

// DownloadOriginal fetches the full-resolution original image for an asset.
func (s *ImmichService) DownloadOriginal(assetID string) ([]byte, error) {
	if err := s.ensureClient(); err != nil {
		return nil, err
	}
	return s.client.DownloadOriginal(assetID)
}

// DownloadPhoto downloads the original full-resolution image and converts it
// to JPEG using ImageMagick (handles HEIC, RAW formats and EXIF auto-orient).
func (s *ImmichService) DownloadPhoto(assetID string) ([]byte, error) {
	data, err := s.DownloadOriginal(assetID)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("imagemagick conversion failed: %w, output: %s", err, string(output))
	}

	return os.ReadFile(outputPath)
}
