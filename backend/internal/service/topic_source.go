package service

// topicSource is the shared base for "search-topic" image sources (ARTIC,
// Unsplash, Pexels). Each user-entered topic is stored as a synced album
// (kind=real); a sync runs the source's search API for each topic and upserts
// the results as images via the shared album-sync engine. The three sources
// differ only in their search closure and whether they need an API key, so
// almost everything lives here.

import (
	"image"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// topicSyncLimit is how many results one topic sync pulls per topic.
const topicSyncLimit = 100

// topicSource implements AlbumSource and the photo-sync backend surface for a
// search-topic source.
type topicSource struct {
	db       *gorm.DB
	settings *SettingsService
	source   string
	// search runs the source's search API for one topic (query) and maps the
	// results to source-agnostic RemoteAssets.
	search func(query string, limit int) ([]RemoteAsset, error)
	// apiKeyName is the settings key holding the source's API key. Empty when
	// requiresKey is false (ARTIC needs no key).
	apiKeyName  string
	requiresKey bool

	autoSync *AutoSyncScheduler

	mu      sync.Mutex
	syncing bool
}

// newTopicSource wires the shared base and its auto-sync scheduler.
func newTopicSource(db *gorm.DB, settings *SettingsService, source, apiKeyName string, requiresKey bool, search func(query string, limit int) ([]RemoteAsset, error)) *topicSource {
	t := &topicSource{
		db:          db,
		settings:    settings,
		source:      source,
		search:      search,
		apiKeyName:  apiKeyName,
		requiresKey: requiresKey,
	}
	t.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:     source,
		Settings: settings,
		IsRelevantKey: func(key string) bool {
			switch key {
			case source + "_auto_sync_enabled", source + "_auto_sync_interval":
				return true
			}
			return requiresKey && key == apiKeyName
		},
		IsConfigured: t.isAutoSyncConfigured,
		GetConfig:    t.getAutoSyncConfig,
		RunSync:      t.clearAndResyncInternal,
	})
	return t
}

// --- AlbumSource ---

// Source implements AlbumSource.
func (t *topicSource) Source() string { return t.source }

// ListRemoteAlbums implements AlbumSource. Topics are named locally (the topic
// text is the album name), so there is nothing remote to list.
func (t *topicSource) ListRemoteAlbums() ([]RemoteAlbum, error) { return nil, nil }

// FetchAlbumAssets implements AlbumSource: run the search API for the topic
// (stored in the album's ExternalID).
func (t *topicSource) FetchAlbumAssets(album model.Album) ([]RemoteAsset, error) {
	return t.search(album.ExternalID, topicSyncLimit)
}

// --- topic management ---

// ImportPhotos syncs every enabled topic into the local DB via the shared
// album-sync engine.
func (t *topicSource) ImportPhotos() error {
	_, err := SyncAlbumSource(t.db, t)
	return err
}

// SetSyncTopics defines the set of topics to sync. Each non-empty (trimmed)
// topic becomes a sync-enabled album; all other topic albums are disabled.
func (t *topicSource) SetSyncTopics(topics []string) error {
	albums := make([]RemoteAlbum, 0, len(topics))
	for _, topic := range topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue
		}
		albums = append(albums, RemoteAlbum{ExternalID: trimmed, Name: trimmed})
	}
	return SetSyncAlbums(t.db, t.source, albums)
}

// --- PhotoSyncBackend surface (mirrors ImmichService) ---

// GetPhotoCount returns the number of locally-stored photos for the source.
func (t *topicSource) GetPhotoCount() (int64, error) {
	return sourcePhotoCount(t.db, t.source)
}

// ClearPhotos deletes all of the source's photos from the database.
func (t *topicSource) ClearPhotos() error {
	return clearSourcePhotos(t.db, t.source)
}

// ClearAndResync clears then re-imports in the background, returning promptly.
func (t *topicSource) ClearAndResync() error {
	t.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether a sync is currently running.
func (t *topicSource) IsSyncing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.syncing || t.autoSync.IsRunning()
}

func (t *topicSource) clearAndResyncInternal() error {
	t.mu.Lock()
	t.syncing = true
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.syncing = false
		t.mu.Unlock()
	}()

	if err := t.ClearPhotos(); err != nil {
		return err
	}
	return t.ImportPhotos()
}

// --- auto-sync ---

// StartAutoSync starts the background auto-sync loop.
func (t *topicSource) StartAutoSync() {
	t.autoSync.Start()
}

// isAutoSyncConfigured reports whether the source is ready to auto-sync: for
// key-requiring sources the API key must be present (ARTIC is always ready).
func (t *topicSource) isAutoSyncConfigured() bool {
	if t.requiresKey {
		key, _ := t.settings.Get(t.apiKeyName)
		if strings.TrimSpace(key) == "" {
			return false
		}
	}
	return true
}

func (t *topicSource) getAutoSyncConfig() (bool, time.Duration) {
	return parseAutoSyncConfig(t.settings,
		t.source+"_auto_sync_enabled", t.source+"_auto_sync_interval")
}

// NewTopicSourcePlugin constructs the shared registry plugin for a topic
// source. All three topic sources store the image URL in FilePath, so one
// plugin implementation serves every source — it just picks a random DB photo
// for the source's albums and fetches the bytes over HTTP.
func NewTopicSourcePlugin(db *gorm.DB, source string) imagesource.Source {
	return &topicSourcePlugin{db: db, source: source}
}

type topicSourcePlugin struct {
	db     *gorm.DB
	source string
}

func (p *topicSourcePlugin) Name() string { return p.source }

func (p *topicSourcePlugin) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	var albumIDs []uint
	if req.Device != nil {
		albumIDs = DeviceAlbumIDs(p.db, req.Device.ID, p.source)
	}
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForAlbums(p.db, p.source, orientation, albumIDs, exclude)
	}
	load := func(item model.Image) (image.Image, error) {
		resp, err := http.Get(item.FilePath)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		img, _, err := image.Decode(resp.Body)
		return img, err
	}
	return RunDBPhotoFlow(req, p.db, pick, load)
}
