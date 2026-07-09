package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
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

// loginRetryCooldown is how long background callers (thumbnail proxying,
// auto-sync) wait after a failed login before trying again. Without it, a
// burst of gallery thumbnail requests against an expired session each retries
// the login serially under s.mu, blocking the explicit Connect/Test request
// for minutes.
const loginRetryCooldown = 30 * time.Second

type SynologyService struct {
	db       *gorm.DB
	settings *SettingsService
	client   *synology.Client
	mu       sync.Mutex
	// lastLoginFail and loginGen are guarded by mu. loginGen counts
	// successful logins so relogin can detect that another request already
	// re-authenticated the session.
	lastLoginFail time.Time
	loginGen      uint64
	autoSync      *AutoSyncScheduler
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
		RunSync:      svc.resyncInternal,
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

// ensureClient initializes and logs in the client if needed. force bypasses
// the failed-login cooldown and is reserved for explicit user actions
// (Connect/Test); background callers fail fast while the cooldown is active.
func (s *SynologyService) ensureClient(otpCode string, force bool) error {
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
		if !force && time.Since(s.lastLoginFail) < loginRetryCooldown {
			return errors.New("synology login recently failed; waiting before retrying")
		}
		if err := s.client.Login(otpCode); err != nil {
			s.lastLoginFail = time.Now()
			return err
		}
		s.lastLoginFail = time.Time{}
		s.loginGen++
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

	return s.ensureClient(otpCode, true)
}

func (s *SynologyService) GetPhoto(id int, cacheKeyStr, size string) ([]byte, error) {
	if err := s.ensureClient("", false); err != nil {
		return nil, err
	}

	// 1. Find photo in DB to get stored cache key. Match on external_id (the
	// synology id as a string), which the shared sync engine populates.
	var img model.Image
	if err := s.db.Where("external_id = ? AND source = ?", strconv.Itoa(id), model.SourceSynologyPhotos).First(&img).Error; err != nil {
		// Fallback if not found in DB
		gen := s.loginGeneration()
		data, getErr := s.client.GetPhoto(id, cacheKeyStr, size, 0, s.client.SynoToken)
		if s.isAuthExpired(getErr) {
			if reErr := s.relogin(gen); reErr != nil {
				return nil, reErr
			}
			return s.client.GetPhoto(id, cacheKeyStr, size, 0, s.client.SynoToken)
		}
		return data, getErr
	}

	// 2. Resolve the album for this photo (membership first, then the legacy
	// global setting) so multi-album photos are fetched with the right album.
	albumID := s.albumIDForImage(img.ID)

	gen := s.loginGeneration()
	data, err := s.client.GetPhoto(id, img.ThumbnailKey, size, albumID, s.client.SynoToken)
	if s.isAuthExpired(err) {
		if reErr := s.relogin(gen); reErr != nil {
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

// loginGeneration returns the current successful-login counter. Callers
// capture it before a client call so relogin can tell whether the session
// that failed is still the current one.
func (s *SynologyService) loginGeneration() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loginGen
}

// relogin clears the expired session and attempts to re-authenticate.
// The saved DID (device token) allows bypassing 2FA on trusted devices.
// gen is the login generation the caller observed before its request failed;
// if another request already re-authenticated since, relogin is a no-op so
// concurrent auth-expired requests (e.g. a burst of gallery thumbnails) share
// a single login instead of each re-logging-in serially.
func (s *SynologyService) relogin(gen uint64) error {
	s.mu.Lock()
	if s.loginGen != gen {
		s.mu.Unlock()
		return nil
	}
	s.client.SID = ""
	s.mu.Unlock()
	s.settings.Set("synology_sid", "")
	log.Printf("Synology session expired, attempting re-login with saved device token")
	return s.ensureClient("", false)
}

func (s *SynologyService) isAuthExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "code: 119")
}

func (s *SynologyService) ListAlbums() ([]synology.Album, error) {
	if err := s.ensureClient("", false); err != nil {
		return nil, err
	}

	gen := s.loginGeneration()
	albums, err := s.client.ListAlbums(0, 100)
	if s.isAuthExpired(err) {
		if reErr := s.relogin(gen); reErr != nil {
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

// Source implements AlbumSource: the model.Source* constant this owns.
func (s *SynologyService) Source() string { return model.SourceSynologyPhotos }

// ListRemoteAlbums implements AlbumSource: the current Synology albums, used for
// a best-effort name refresh during sync.
func (s *SynologyService) ListRemoteAlbums() ([]RemoteAlbum, error) {
	albums, err := s.ListAlbums()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteAlbum, 0, len(albums))
	for _, a := range albums {
		out = append(out, RemoteAlbum{ExternalID: strconv.Itoa(a.ID), Name: a.Name})
	}
	return out, nil
}

// FetchAlbumAssets implements AlbumSource: pages through one Synology album,
// backfills any missing resolutions (0x0 → decode a thumbnail), then maps each
// photo to a source-agnostic RemoteAsset.
func (s *SynologyService) FetchAlbumAssets(album model.Album) ([]RemoteAsset, error) {
	albumID, err := strconv.Atoi(album.ExternalID)
	if err != nil {
		return nil, err
	}

	// Page through the album over the network.
	var photos []synology.Item
	offset, limit := 0, 500
	for offset < 5000 {
		gen := s.loginGeneration()
		batch, e := s.client.ListPhotos(offset, limit, albumID)
		if s.isAuthExpired(e) {
			if reErr := s.relogin(gen); reErr != nil {
				return nil, reErr
			}
			batch, e = s.client.ListPhotos(offset, limit, albumID)
		}
		if e != nil {
			return nil, e
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

	// Synology omits resolution for some items (reported as 0x0), which
	// determineOrientation would misclassify as landscape. Recover the real
	// orientation by decoding a thumbnail. Runs outside any DB transaction.
	s.backfillMissingResolutions(photos, albumID)

	out := make([]RemoteAsset, 0, len(photos))
	for _, p := range photos {
		pw, ph := p.Additional.Resolution.Width, p.Additional.Resolution.Height
		thumbKey := p.Additional.Thumbnail.M
		if p.Additional.Thumbnail.XL != "" {
			thumbKey = p.Additional.Thumbnail.XL
		}
		var photoTaken *time.Time
		if p.Time > 0 {
			t := time.Unix(p.Time, 0)
			photoTaken = &t
		}
		out = append(out, RemoteAsset{
			ExternalID:   strconv.Itoa(p.ID),
			FilePath:     p.Filename,
			Width:        pw,
			Height:       ph,
			Orientation:  determineOrientation(pw, ph, ""),
			ThumbnailKey: thumbKey,
			PhotoTakenAt: photoTaken,
		})
	}
	return out, nil
}

// ImportPhotos syncs every sync-enabled Synology album into the DB via the
// shared album-sync engine: image rows (deduped by external_id) plus (asset,
// album) memberships, pruning stale memberships and orphaned image rows.
func (s *SynologyService) ImportPhotos() error {
	if err := s.ensureClient("", false); err != nil {
		return err
	}

	s.ensureGlobalAlbumSeed()

	_, err := SyncAlbumSource(s.db, s)
	return err
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

// resolvePhotoDimensions decodes a thumbnail to recover an image's dimensions
// when Synology's list response omits the resolution. A thumbnail preserves the
// aspect ratio, so this yields the correct orientation (the returned size is the
// thumbnail's, which is all determineOrientation needs). DecodeConfig reads only
// the header, so it stays cheap.
func (s *SynologyService) resolvePhotoDimensions(id int, thumbKey string, albumID int) (int, int, error) {
	if thumbKey == "" {
		return 0, 0, errors.New("no thumbnail key")
	}
	gen := s.loginGeneration()
	data, err := s.client.GetPhoto(id, thumbKey, "large", albumID, s.client.SynoToken)
	if s.isAuthExpired(err) {
		if reErr := s.relogin(gen); reErr != nil {
			return 0, 0, reErr
		}
		data, err = s.client.GetPhoto(id, thumbKey, "large", albumID, s.client.SynoToken)
	}
	if err != nil {
		return 0, 0, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// backfillMissingResolutions fills width/height for photos Synology returned
// without a resolution (0x0) by decoding a thumbnail, mutating photos in place.
// Runs outside any DB transaction (network I/O). Photos a previous sync already
// resolved are skipped, so it's a one-time cost per photo.
func (s *SynologyService) backfillMissingResolutions(photos []synology.Item, albumID int) {
	var needIDs []int
	for _, p := range photos {
		if p.Additional.Resolution.Width <= 0 || p.Additional.Resolution.Height <= 0 {
			needIDs = append(needIDs, p.ID)
		}
	}
	if len(needIDs) == 0 {
		return
	}
	// Skip ones a previous sync already resolved (real dims stored). Match on
	// external_id (the synology id as a string), which the shared sync engine
	// populates.
	needExtIDs := make([]string, len(needIDs))
	for i, id := range needIDs {
		needExtIDs[i] = strconv.Itoa(id)
	}
	var resolvedExtIDs []string
	s.db.Model(&model.Image{}).
		Where("source = ? AND external_id IN ? AND width > 0 AND height > 0",
			model.SourceSynologyPhotos, needExtIDs).
		Pluck("external_id", &resolvedExtIDs)
	resolved := make(map[int]bool, len(resolvedExtIDs))
	for _, ext := range resolvedExtIDs {
		if n, err := strconv.Atoi(ext); err == nil {
			resolved[n] = true
		}
	}
	for i := range photos {
		p := &photos[i]
		if p.Additional.Resolution.Width > 0 && p.Additional.Resolution.Height > 0 {
			continue
		}
		if resolved[p.ID] {
			continue
		}
		thumbKey := p.Additional.Thumbnail.M
		if thumbKey == "" {
			thumbKey = p.Additional.Thumbnail.XL
		}
		w, h, err := s.resolvePhotoDimensions(p.ID, thumbKey, albumID)
		if err != nil || w <= 0 || h <= 0 {
			log.Printf("synology: could not decode dimensions for photo %d: %v", p.ID, err)
			continue
		}
		p.Additional.Resolution.Width = w
		p.Additional.Resolution.Height = h
		// Persist onto an existing row too: the shared upsert engine only inserts
		// new rows, so without this a previously-imported 0x0 row would never
		// self-correct (and we'd re-decode it every sync).
		if e := s.db.Model(&model.Image{}).
			Where("source = ? AND external_id = ? AND (width <= 0 OR height <= 0)",
				model.SourceSynologyPhotos, strconv.Itoa(p.ID)).
			Updates(map[string]interface{}{
				"width":       w,
				"height":      h,
				"orientation": determineOrientation(w, h, ""),
			}).Error; e != nil {
			log.Printf("synology: update dims for existing photo %d: %v", p.ID, e)
		}
	}
}

// SetSyncAlbums defines which Synology albums (by external id) to sync: enables
// those album rows and disables the rest via the shared engine. Synology has
// only real albums, so no virtual-album handling is needed. Persists the
// selection only — the import is triggered explicitly (manual Sync) or by the
// auto-sync scheduler, not on every album toggle.
func (s *SynologyService) SetSyncAlbums(realIDs []string) error {
	nameByID := map[string]string{}
	if albums, err := s.ListAlbums(); err == nil {
		for _, a := range albums {
			nameByID[strconv.Itoa(a.ID)] = a.Name
		}
	}

	albums := make([]RemoteAlbum, 0, len(realIDs))
	for _, id := range realIDs {
		if id == "" {
			continue
		}
		name := nameByID[id]
		if name == "" {
			name = id
		}
		albums = append(albums, RemoteAlbum{ExternalID: id, Name: name})
	}
	return SetSyncAlbums(s.db, model.SourceSynologyPhotos, albums)
}

// ClearPhotos deletes all Synology photos and their album memberships.
func (s *SynologyService) ClearPhotos() error {
	return clearSourcePhotos(s.db, model.SourceSynologyPhotos)
}

// ClearAndResync runs an incremental resync from the configured albums. Despite
// the name (kept for the PhotoSyncBackend interface), it does NOT clear first —
// see resyncInternal.
func (s *SynologyService) ClearAndResync() error {
	// Run the resync in the background so the manual Sync request returns
	// promptly; the client polls sync-status to show progress.
	s.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether a Synology sync is currently running.
func (s *SynologyService) IsSyncing() bool {
	return s.autoSync.IsRunning()
}

// resyncInternal runs an incremental sync (upsert + prune) via ImportPhotos. It
// deliberately does NOT clear first: the auto-sync scheduler fires this on every
// startup (lastSuccessAt is in-memory and resets to zero), and a blanket delete
// before a re-import that then fails would leave the gallery empty. Removed
// assets are pruned by the shared album-sync engine; the explicit Clear button
// (ClearPhotos) remains the only destructive path.
func (s *SynologyService) resyncInternal() error {
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
