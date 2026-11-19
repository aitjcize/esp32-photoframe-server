package service

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/immich"
	"gorm.io/gorm"
)

// ImmichService talks to one Immich server: the connection row it was built
// for. Configuration is read from the row on every use, so an edit takes
// effect on the next call; the client is rebuilt when URL or key change.
type ImmichService struct {
	db       *gorm.DB
	conns    *ConnectionStore
	id       uint
	client   *immich.Client
	mu       sync.Mutex
	autoSync *AutoSyncScheduler
}

// NewImmichService builds the service for connection id.
func NewImmichService(db *gorm.DB, conns *ConnectionStore, id uint) *ImmichService {
	svc := &ImmichService{db: db, conns: conns, id: id}
	svc.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:         fmt.Sprintf("Immich#%d", id),
		IsConfigured: svc.isAutoSyncConfigured,
		GetConfig:    svc.getAutoSyncConfig,
		RunSync:      svc.resyncInternal,
	})
	return svc
}

// StartAutoSync starts a background loop that periodically syncs Immich photos
// when the connection's auto-sync is enabled.
func (s *ImmichService) StartAutoSync() {
	s.autoSync.Start()
}

// Stop ends the auto-sync loop; used when the connection is deleted.
func (s *ImmichService) Stop() {
	s.autoSync.Stop()
}

// Reconfigure is called after the connection row changed: drop the cached
// client so the next call rebuilds it, and re-arm the auto-sync timer.
func (s *ImmichService) Reconfigure() {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	s.autoSync.TriggerReset()
}

// ConnectionID implements AlbumSource.
func (s *ImmichService) ConnectionID() uint { return s.id }

// scope is this connection's album/image pool.
func (s *ImmichService) scope() sourceScope {
	return sourceScope{Source: model.SourceImmich, ConnectionID: s.id}
}

// config loads the connection row and its decoded config.
func (s *ImmichService) config() (model.Connection, model.ImmichConfig, error) {
	c, err := s.conns.Get(s.id)
	if err != nil {
		return c, model.ImmichConfig{}, err
	}
	return c, c.ImmichConfig(), nil
}

// Immich memory modes — how the "memories" source picks lanes. See issue #32.
const (
	ImmichMemoryModeAll    = "all"    // flatten all "on this day" lanes across years (default)
	ImmichMemoryModeLatest = "latest" // only the most recent year's lane
)

// immichMemoryMode returns the configured memories lane selection, defaulting
// to all (every year flattened into one pool).
func (s *ImmichService) immichMemoryMode() string {
	if _, cfg, err := s.config(); err == nil && cfg.MemoryMode == ImmichMemoryModeLatest {
		return ImmichMemoryModeLatest
	}
	return ImmichMemoryModeAll
}

func (s *ImmichService) isAutoSyncConfigured() bool {
	c, cfg, err := s.config()
	if err != nil || !c.Enabled || cfg.URL == "" || cfg.APIKey == "" {
		return false
	}
	// The album picker owns the selection: no rows means nothing has been
	// picked yet, rows with none enabled means everything was deselected.
	// (The legacy single-album settings were turned into a row by the
	// connections migration, so there is no settings fallback any more.)
	_, enabled := countAlbums(s.db, s.scope())
	return enabled > 0
}

func (s *ImmichService) getAutoSyncConfig() (bool, time.Duration) {
	_, cfg, err := s.config()
	if err != nil {
		return false, time.Hour
	}
	return autoSyncConfig(cfg.AutoSyncEnabled, cfg.AutoSyncIntervalMinutes)
}

// getClient returns the current client, initializing from stored settings if needed.
// The returned client pointer is safe to use even if s.client is later replaced.
func (s *ImmichService) getClient() (*immich.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" || cfg.APIKey == "" {
		return nil, errors.New("immich credentials not configured")
	}

	if s.client == nil || s.client.BaseURL != cfg.URL || s.client.APIKey != cfg.APIKey {
		s.client = immich.NewClient(cfg.URL, cfg.APIKey)
	}
	return s.client, nil
}

// TestConnection creates a fresh client from the connection and verifies connectivity
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

	err := setSyncAlbumsWithVirtual(s.db, s.scope(), realIDs, nameByID, VirtualSelection{
		All: all, Favorites: favorites, Memories: memories,
	}, "Memories")
	if err != nil {
		return err
	}

	// The scheduler only re-arms on a connection change, and the picker writes
	// album rows -- so without this nudge a selection made while the timer
	// sits on its 24h "not configured" delay wouldn't be picked up until the
	// next restart (issue #54).
	s.autoSync.TriggerReset()

	// Persist the selection only — do NOT import here. The import is triggered
	// explicitly (manual Sync) or by the auto-sync scheduler, so toggling
	// albums doesn't kick off a clear+resync on every change.
	return nil
}

// Source implements AlbumSource: the model.Source* constant this owns.
func (s *ImmichService) Source() string { return model.SourceImmich }

// ListRemoteAlbums implements AlbumSource: the current real albums, used for a
// best-effort name refresh during sync.
func (s *ImmichService) ListRemoteAlbums() ([]RemoteAlbum, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	list, err := client.ListAlbums()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteAlbum, 0, len(list))
	for _, a := range list {
		out = append(out, RemoteAlbum{ExternalID: a.ID, Name: a.AlbumName})
	}
	return out, nil
}

// FetchAlbumAssets implements AlbumSource: the servable IMAGE assets for one
// album (real or virtual), mapped to source-agnostic RemoteAssets. RAW assets
// are skipped (no preview/thumbnail API).
func (s *ImmichService) FetchAlbumAssets(album model.Album) ([]RemoteAsset, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	assets, err := s.fetchAssetsForAlbum(client, album)
	if err != nil {
		return nil, err
	}
	out := make([]RemoteAsset, 0, len(assets))
	for _, a := range assets {
		if !immich.IsServableImage(a) {
			continue
		}
		w, h := a.ExifInfo.ExifImageWidth, a.ExifInfo.ExifImageHeight
		photoDate := parseImmichDate(a.ExifInfo.DateTimeOriginal)
		if photoDate == nil {
			photoDate = parseImmichDate(a.LocalDateTime)
		}
		out = append(out, RemoteAsset{
			ExternalID:   a.ID,
			FilePath:     a.OriginalFileName,
			Width:        w,
			Height:       h,
			Orientation:  determineOrientation(w, h, a.ExifInfo.Orientation),
			PhotoTakenAt: photoDate,
		})
	}
	return out, nil
}

// ImportPhotos syncs every sync-enabled Immich album into the local DB via the
// shared album-sync engine: one images row per asset (deduped by external_id)
// plus an image_album_memberships row per (asset, album). Stale memberships and
// orphaned image rows are pruned so the local pool tracks Immich.
func (s *ImmichService) ImportPhotos() error {
	if _, err := s.getClient(); err != nil {
		return err
	}
	_, err := SyncAlbumSource(s.db, s)
	return err
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

// ClearPhotos deletes all Immich photos from the database
func (s *ImmichService) ClearPhotos() error {
	return clearSourcePhotos(s.db, s.scope())
}

// ClearAndResync runs an incremental resync from the configured albums. Despite
// the name (kept for the PhotoSyncBackend interface), it does NOT clear first —
// see resyncInternal.
func (s *ImmichService) ClearAndResync() error {
	// Run the resync in the background so the manual Sync request returns
	// promptly; the client polls sync-status to show progress.
	s.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether an Immich sync is currently running.
func (s *ImmichService) IsSyncing() bool {
	return s.autoSync.IsRunning()
}

// LastSyncError reports the most recent sync run's failure ("" on success).
func (s *ImmichService) LastSyncError() string {
	return s.autoSync.LastError()
}

// resyncInternal runs an incremental sync (upsert + prune) via ImportPhotos. It
// deliberately does NOT clear first: the auto-sync scheduler fires this on every
// startup (lastSuccessAt is in-memory and resets to zero), and a blanket delete
// before a re-import that then fails would leave the gallery empty. Removed
// assets are pruned by the shared album-sync engine; the explicit Clear button
// (ClearPhotos) remains the only destructive path.
func (s *ImmichService) resyncInternal() error {
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
	return sourcePhotoCount(s.db, s.scope())
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

	out, err := convertToJPEG(data)
	if err != nil {
		log.Printf("Immich asset %s: %v, falling back to preview", assetID, err)
		return s.downloadPreviewFallback(assetID, err)
	}
	return out, nil
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
