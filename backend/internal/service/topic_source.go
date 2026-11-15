package service

// TopicSource is the shared implementation of the "search-topic" sources
// (Unsplash and Pexels), one instance per connection: the connection's config
// holds the API key, each user-entered topic is stored as a synced album
// (kind=real) of the connection, and a sync runs the source's search API for
// each topic and upserts the results as images via the shared album-sync
// engine. The two sources differ only in their search function.

import (
	"errors"
	"fmt"
	"image"
	"math/rand/v2"
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

// topicRandomizeMaxResults caps how deep into the search results randomized
// sampling reaches. Results beyond the first ~1000 drift away from the topic,
// so sampling stays within this window.
const topicRandomizeMaxResults = 1000

// searchTopicPages collects up to limit assets from a paged search API.
// fetch returns one 1-based page of results plus the query's total result
// count. Page 1 is always fetched first — it anchors the most relevant
// results and reveals how many pages exist. The remaining pages are read
// sequentially, or, when randomize is on, sampled in random order from the
// first topicRandomizeMaxResults results so each sync yields a different set.
// Results are deduped by ExternalID (page contents can shift between
// requests).
func searchTopicPages(perPage, limit int, randomize bool, fetch func(page int) ([]RemoteAsset, int, error)) ([]RemoteAsset, error) {
	first, total, err := fetch(1)
	if err != nil {
		return nil, err
	}

	assets := make([]RemoteAsset, 0, limit)
	seen := make(map[string]bool, limit)
	add := func(batch []RemoteAsset) {
		for _, a := range batch {
			if len(assets) >= limit {
				return
			}
			if a.ExternalID == "" || seen[a.ExternalID] {
				continue
			}
			seen[a.ExternalID] = true
			assets = append(assets, a)
		}
	}
	add(first)
	// A short first page is also the last page — nothing more to fetch.
	if len(assets) >= limit || len(first) < perPage {
		return assets, nil
	}

	if randomize && total > perPage {
		lastPage := (min(total, topicRandomizeMaxResults) + perPage - 1) / perPage
		pages := make([]int, 0, lastPage-1)
		for p := 2; p <= lastPage; p++ {
			pages = append(pages, p)
		}
		rand.Shuffle(len(pages), func(i, j int) { pages[i], pages[j] = pages[j], pages[i] })
		for _, p := range pages {
			if len(assets) >= limit {
				break
			}
			batch, _, err := fetch(p)
			if err != nil {
				return nil, err
			}
			add(batch)
		}
		return assets, nil
	}

	for page := 2; len(assets) < limit; page++ {
		batch, _, err := fetch(page)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		before := len(assets)
		add(batch)
		if len(assets) == before {
			break // page yielded nothing new; stop rather than loop forever
		}
	}
	return assets, nil
}

// topicSearchFunc runs a source's search API for one topic with the given
// key and maps the results to source-agnostic RemoteAssets.
type topicSearchFunc func(apiKey string, randomize bool, query string, limit int) ([]RemoteAsset, error)

// topicSearchers maps a topic source to its search function.
var topicSearchers = map[string]topicSearchFunc{
	model.SourceUnsplash: unsplashSearch,
	model.SourcePexels:   pexelsSearch,
}

// TopicSource implements AlbumSource and the photo-sync backend surface for
// one Unsplash or Pexels connection.
type TopicSource struct {
	db     *gorm.DB
	conns  *ConnectionStore
	id     uint
	source string
	search topicSearchFunc

	autoSync *AutoSyncScheduler

	mu      sync.Mutex
	syncing bool
}

// NewTopicSource builds the service for connection id of the given topic
// source and wires its auto-sync scheduler.
func NewTopicSource(db *gorm.DB, conns *ConnectionStore, id uint, source string) (*TopicSource, error) {
	search, ok := topicSearchers[source]
	if !ok {
		return nil, fmt.Errorf("source %q is not a topic source", source)
	}
	t := &TopicSource{db: db, conns: conns, id: id, source: source, search: search}
	t.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:         fmt.Sprintf("%s#%d", source, id),
		IsConfigured: t.isAutoSyncConfigured,
		GetConfig:    t.getAutoSyncConfig,
		RunSync:      t.resyncInternal,
	})
	return t, nil
}

// StartAutoSync starts the background auto-sync loop.
func (t *TopicSource) StartAutoSync() { t.autoSync.Start() }

// Stop ends the auto-sync loop; used when the connection is deleted.
func (t *TopicSource) Stop() { t.autoSync.Stop() }

// Reconfigure re-arms the auto-sync timer after the connection row changed.
func (t *TopicSource) Reconfigure() { t.autoSync.TriggerReset() }

// config loads the connection row and its decoded config.
func (t *TopicSource) config() (model.Connection, model.TopicConfig, error) {
	c, err := t.conns.Get(t.id)
	if err != nil {
		return c, model.TopicConfig{}, err
	}
	return c, c.TopicConfig(), nil
}

// --- AlbumSource ---

// Source implements AlbumSource.
func (t *TopicSource) Source() string { return t.source }

// ConnectionID implements AlbumSource.
func (t *TopicSource) ConnectionID() uint { return t.id }

// scope is this connection's album/image pool.
func (t *TopicSource) scope() sourceScope {
	return sourceScope{Source: t.source, ConnectionID: t.id}
}

// ListRemoteAlbums implements AlbumSource. Topics are named locally (the topic
// text is the album name), so there is nothing remote to list.
func (t *TopicSource) ListRemoteAlbums() ([]RemoteAlbum, error) { return nil, nil }

// FetchAlbumAssets implements AlbumSource: run the search API for the topic
// (stored in the album's ExternalID) with the connection's key.
func (t *TopicSource) FetchAlbumAssets(album model.Album) ([]RemoteAsset, error) {
	_, cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	if cfg.APIKey == "" {
		return nil, errors.New(t.source + " api key not configured")
	}
	return t.search(cfg.APIKey, cfg.RandomizeResults, album.ExternalID, topicSyncLimit)
}

// --- topic management ---

// ImportPhotos syncs every enabled topic into the local DB via the shared
// album-sync engine.
func (t *TopicSource) ImportPhotos() error {
	_, err := SyncAlbumSource(t.db, t)
	return err
}

// SetSyncTopics makes the connection's synced topic set EXACTLY the given
// topics: it creates new topic albums, keeps existing ones enabled, and DELETES
// removed ones (with their memberships), then GCs any now-orphaned images.
// Unlike remote-album sources (Immich/Synology), a removed topic is gone for
// good rather than merely disabled -- there is nothing remote to re-enable, and
// a disabled row would otherwise reappear in the topic list.
func (t *TopicSource) SetSyncTopics(topics []string) error {
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

	sc := t.scope()
	err := t.db.Transaction(func(tx *gorm.DB) error {
		// Delete topic albums no longer wanted, plus their memberships.
		q := sc.where(tx)
		if len(want) > 0 {
			q = q.Where("external_id NOT IN ?", want)
		}
		var stale []model.Album
		if e := q.Find(&stale).Error; e != nil {
			return e
		}
		for _, a := range stale {
			// Album has no soft-delete, so this is a real DELETE and the album's
			// image_album_memberships drop via ON DELETE CASCADE; the now-orphaned
			// images are swept by gcOrphanImagesForSource below.
			if e := tx.Delete(&model.Album{}, a.ID).Error; e != nil {
				return e
			}
		}
		// Upsert wanted topics as sync-enabled real albums.
		for _, topic := range want {
			var existing model.Album
			e := sc.where(tx).Where("external_id = ?", topic).First(&existing).Error
			switch {
			case e == gorm.ErrRecordNotFound:
				if ce := tx.Create(&model.Album{
					Source: t.source, ExternalID: topic, Name: topic,
					Kind: model.AlbumKindReal, SyncEnabled: true,
					ConnectionID: sc.connectionRef(),
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
	gcOrphanImagesForSource(t.db, sc)
	return nil
}

// Topics lists the connection's synced topics, oldest first.
func (t *TopicSource) Topics() ([]string, error) {
	var albums []model.Album
	if err := t.scope().where(t.db.Model(&model.Album{})).Order("id").Find(&albums).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(albums))
	for _, a := range albums {
		out = append(out, a.ExternalID)
	}
	return out, nil
}

// TestConnection validates the connection's API key by running a trivial
// search.
func (t *TopicSource) TestConnection() error {
	_, cfg, err := t.config()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("api key not configured")
	}
	if _, err := t.search(cfg.APIKey, false, "landscape", 1); err != nil {
		return err
	}
	return nil
}

// --- PhotoSyncBackend surface (mirrors ImmichService) ---

// GetPhotoCount returns the number of photos synced into this connection.
func (t *TopicSource) GetPhotoCount() (int64, error) {
	return sourcePhotoCount(t.db, t.scope())
}

// ClearPhotos deletes all of the connection's photos from the database.
func (t *TopicSource) ClearPhotos() error {
	return clearSourcePhotos(t.db, t.scope())
}

// ClearAndResync runs an incremental resync in the background, returning
// promptly. Despite the name (kept for the PhotoSyncBackend interface), it does
// NOT clear first — see resyncInternal.
func (t *TopicSource) ClearAndResync() error {
	t.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether a sync is currently running.
func (t *TopicSource) IsSyncing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.syncing || t.autoSync.IsRunning()
}

// LastSyncError reports the most recent sync run's failure ("" on success).
func (t *TopicSource) LastSyncError() string {
	return t.autoSync.LastError()
}

// resyncInternal runs an incremental sync: it upserts current results and lets
// the shared album-sync engine prune assets that disappeared upstream. It does
// NOT clear first — a blanket delete before a re-import that can fail (Unsplash
// rate limits, transient network, a key not yet available at startup) would
// wipe the gallery and leave it empty. This is the auto-sync/manual-sync path;
// the explicit Clear button (ClearPhotos) is the only destructive one.
func (t *TopicSource) resyncInternal() error {
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

// isAutoSyncConfigured reports whether the connection is ready to auto-sync:
// enabled, with an API key and at least one topic.
func (t *TopicSource) isAutoSyncConfigured() bool {
	c, cfg, err := t.config()
	if err != nil || !c.Enabled || strings.TrimSpace(cfg.APIKey) == "" {
		return false
	}
	_, enabled := countAlbums(t.db, t.scope())
	return enabled > 0
}

func (t *TopicSource) getAutoSyncConfig() (bool, time.Duration) {
	_, cfg, err := t.config()
	if err != nil {
		return false, 0
	}
	return autoSyncConfig(cfg.AutoSyncEnabled, cfg.AutoSyncIntervalMinutes)
}

// NewTopicSourcePlugin constructs the shared registry plugin for a topic
// source. Both topic sources store the image URL in FilePath, so one plugin
// implementation serves every source — it just picks a random DB photo for
// the source's albums (of the frame's connection, or all of them) and fetches the bytes
// over HTTP.
func NewTopicSourcePlugin(db *gorm.DB, source string) imagesource.Source {
	return &topicSourcePlugin{db: db, source: source}
}

type topicSourcePlugin struct {
	db     *gorm.DB
	source string
}

func (p *topicSourcePlugin) Name() string { return p.source }

func (p *topicSourcePlugin) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForDevice(p.db, p.source, req.Device, orientation, exclude)
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
