package service

// topicSource is the shared base for "search-topic" image sources (Unsplash
// and Pexels). Each user-entered topic is stored as a synced album
// (kind=real); a sync runs the source's search API for each topic and upserts
// the results as images via the shared album-sync engine. The three sources
// differ only in their search closure and whether they need an API key, so
// almost everything lives here.

import (
	"errors"
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
	// requiresKey is false.
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
		RunSync:      t.resyncInternal,
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

// SetSyncTopics makes the synced topic set EXACTLY the given topics: it creates
// new topic albums, keeps existing ones enabled, and DELETES removed ones (with
// their memberships), then GCs any now-orphaned images. Unlike remote-album
// sources (Immich/Synology), a removed topic is gone for good rather than merely
// disabled -- there is nothing remote to re-enable, and a disabled row would
// otherwise reappear in the topic list.
func (t *topicSource) SetSyncTopics(topics []string) error {
	seen := map[string]bool{}
	want := make([]string, 0, len(topics))
	for _, topic := range topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		want = append(want, trimmed)
	}

	err := t.db.Transaction(func(tx *gorm.DB) error {
		// Delete topic albums no longer wanted, plus their memberships.
		q := tx.Where("source = ?", t.source)
		if len(want) > 0 {
			q = q.Where("external_id NOT IN ?", want)
		}
		var stale []model.Album
		if e := q.Find(&stale).Error; e != nil {
			return e
		}
		for _, a := range stale {
			if e := tx.Where("album_id = ?", a.ID).Delete(&model.ImageAlbumMembership{}).Error; e != nil {
				return e
			}
			if e := tx.Delete(&model.Album{}, a.ID).Error; e != nil {
				return e
			}
		}
		// Upsert wanted topics as sync-enabled real albums.
		for _, topic := range want {
			var existing model.Album
			e := tx.Where("source = ? AND external_id = ?", t.source, topic).First(&existing).Error
			switch {
			case e == gorm.ErrRecordNotFound:
				if ce := tx.Create(&model.Album{
					Source: t.source, ExternalID: topic, Name: topic,
					Kind: model.AlbumKindReal, SyncEnabled: true,
				}).Error; ce != nil {
					return ce
				}
			case e == nil:
				if ue := tx.Model(&model.Album{}).Where("id = ?", existing.ID).
					Update("sync_enabled", true).Error; ue != nil {
					return ue
				}
			default:
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Drop images orphaned by the deleted topic albums.
	gcOrphanImagesForSource(t.db, t.source)
	return nil
}

// TestConnection validates the source's API key by running a trivial search.
// Key-less sources are always considered connected.
func (t *topicSource) TestConnection() error {
	if !t.requiresKey {
		return nil
	}
	if key, _ := t.settings.Get(t.apiKeyName); strings.TrimSpace(key) == "" {
		return errors.New("api key not configured")
	}
	if _, err := t.search("landscape", 1); err != nil {
		return err
	}
	return nil
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

// ClearAndResync runs an incremental resync in the background, returning
// promptly. Despite the name (kept for the PhotoSyncBackend interface), it does
// NOT clear first — see resyncInternal.
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

// resyncInternal runs an incremental sync: it upserts current results and lets
// the shared album-sync engine prune assets that disappeared upstream. It does
// NOT clear first — a blanket delete before a re-import that can fail (Unsplash
// rate limits, transient network, a key not yet available at startup) would
// wipe the gallery and leave it empty. This is the auto-sync/manual-sync path;
// the explicit Clear button (ClearPhotos) is the only destructive one.
func (t *topicSource) resyncInternal() error {
	t.mu.Lock()
	t.syncing = true
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.syncing = false
		t.mu.Unlock()
	}()

	return t.ImportPhotos()
}

// --- auto-sync ---

// StartAutoSync starts the background auto-sync loop.
func (t *topicSource) StartAutoSync() {
	t.autoSync.Start()
}

// isAutoSyncConfigured reports whether the source is ready to auto-sync: for
// key-requiring sources the API key must be present; key-less ones are ready.
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
