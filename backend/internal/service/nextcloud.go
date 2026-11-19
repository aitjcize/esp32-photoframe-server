package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/nextcloud"
	"gorm.io/gorm"
)

// NextcloudService talks to the Memories app of one Nextcloud server as one
// user: the connection row it was built for. Like ImmichService it syncs real
// albums plus the virtual All Photos / Favorites / On This Day collections
// through the shared album-sync engine, and reads its configuration from the
// row on every use.
type NextcloudService struct {
	db       *gorm.DB
	conns    *ConnectionStore
	id       uint
	client   *nextcloud.Client
	mu       sync.Mutex
	autoSync *AutoSyncScheduler

	// The Login Flow v2 sign-in in progress or last finished (guarded by mu).
	login *nextcloudLogin
	// For tests: the login flow's HTTP client and poll cadence.
	loginHTTP     *http.Client
	loginPoll     time.Duration
	loginDeadline time.Duration
}

// Login states reported by LoginStatus.
const (
	NextcloudLoginIdle    = "idle"    // no sign-in started since the server started
	NextcloudLoginPending = "pending" // waiting for the user to grant access
	NextcloudLoginDone    = "done"    // credentials stored
	NextcloudLoginFailed  = "failed"  // expired, cancelled or refused
)

type nextcloudLogin struct {
	state    string
	loginURL string
	err      string
	cancel   context.CancelFunc
}

// NextcloudLoginStatus is the sign-in progress the dashboard polls.
type NextcloudLoginStatus struct {
	State    string `json:"state"`
	LoginURL string `json:"login_url,omitempty"`
	Error    string `json:"error,omitempty"`
}

// NewNextcloudService builds the service for connection id.
func NewNextcloudService(db *gorm.DB, conns *ConnectionStore, id uint) *NextcloudService {
	svc := &NextcloudService{
		db: db, conns: conns, id: id,
		loginPoll:     2 * time.Second,
		loginDeadline: 20 * time.Minute, // Nextcloud expires a flow after 20 minutes
	}
	svc.autoSync = NewAutoSyncScheduler(AutoSyncSchedulerOptions{
		Name:         fmt.Sprintf("Nextcloud#%d", id),
		IsConfigured: svc.isAutoSyncConfigured,
		GetConfig:    svc.getAutoSyncConfig,
		RunSync:      svc.ImportPhotos,
	})
	return svc
}

// StartAutoSync starts the background sync loop.
func (s *NextcloudService) StartAutoSync() { s.autoSync.Start() }

// Stop ends the auto-sync loop and any sign-in in progress; used when the
// connection is deleted.
func (s *NextcloudService) Stop() {
	s.mu.Lock()
	if s.login != nil && s.login.cancel != nil {
		s.login.cancel()
	}
	s.mu.Unlock()
	s.autoSync.Stop()
}

// BeginLogin starts Nextcloud's Login Flow v2 against the connection's URL
// and returns the page the user signs in on. A background poll stores the
// user name and the app password Nextcloud creates once access is granted.
// Starting again abandons an earlier flow.
func (s *NextcloudService) BeginLogin() (string, error) {
	_, cfg, err := s.config()
	if err != nil {
		return "", err
	}
	if cfg.URL == "" {
		return "", errors.New("enter the Nextcloud URL first")
	}
	hc := s.loginHTTP
	if hc == nil {
		hc = nextcloud.NewLoginHTTPClient()
	}
	flow, err := nextcloud.StartLogin(hc, cfg.URL)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.loginDeadline)
	l := &nextcloudLogin{state: NextcloudLoginPending, loginURL: flow.LoginURL, cancel: cancel}
	s.mu.Lock()
	if s.login != nil && s.login.cancel != nil {
		s.login.cancel()
	}
	s.login = l
	s.mu.Unlock()

	go s.pollLogin(ctx, hc, cfg.URL, flow.Token, l)
	return flow.LoginURL, nil
}

// pollLogin waits for the user to grant access, then stores the credentials.
func (s *NextcloudService) pollLogin(ctx context.Context, hc *http.Client, baseURL, token string, l *nextcloudLogin) {
	finish := func(state, msg string) {
		s.mu.Lock()
		if s.login == l {
			l.state, l.err = state, msg
		}
		s.mu.Unlock()
	}
	ticker := time.NewTicker(s.loginPoll)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			msg := "the sign-in expired; start it again"
			if errors.Is(ctx.Err(), context.Canceled) {
				msg = "the sign-in was cancelled"
			} else if lastErr != nil {
				msg += " (last error: " + lastErr.Error() + ")"
			}
			finish(NextcloudLoginFailed, msg)
			return
		case <-ticker.C:
		}
		creds, err := nextcloud.PollLogin(hc, baseURL, token)
		if err != nil {
			// Transient (network, proxy): keep polling until the deadline.
			lastErr = err
			continue
		}
		if creds == nil {
			continue
		}
		if err := s.storeCredentials(baseURL, creds); err != nil {
			finish(NextcloudLoginFailed, "could not save the credentials: "+err.Error())
		} else {
			finish(NextcloudLoginDone, "")
		}
		l.cancel()
		return
	}
}

// storeCredentials saves what the login flow handed over, unless the URL was
// changed meanwhile (the credentials belong to the server that issued them).
func (s *NextcloudService) storeCredentials(baseURL string, creds *nextcloud.Credentials) error {
	c, cfg, err := s.config()
	if err != nil {
		return err
	}
	if cfg.URL != baseURL {
		return errors.New("the Nextcloud URL changed during sign-in")
	}
	cfg.Username = creds.LoginName
	cfg.AppPassword = creds.AppPassword
	if err := c.EncodeConfig(cfg); err != nil {
		return err
	}
	c.Enabled = true
	if err := s.conns.Update(&c); err != nil {
		return err
	}
	s.Reconfigure()
	return nil
}

// LoginStatus reports the sign-in in progress or last finished.
func (s *NextcloudService) LoginStatus() NextcloudLoginStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.login == nil {
		return NextcloudLoginStatus{State: NextcloudLoginIdle}
	}
	return NextcloudLoginStatus{State: s.login.state, LoginURL: s.login.loginURL, Error: s.login.err}
}

// Logout revokes the app password on the Nextcloud (best-effort: the server
// may be gone) and forgets the credentials. The URL, album selection and
// synced photos stay; signing in again resumes syncing.
func (s *NextcloudService) Logout() error {
	c, cfg, err := s.config()
	if err != nil {
		return err
	}
	if cfg.AppPassword != "" {
		if client, err := s.getClient(); err == nil {
			if err := client.RevokeAppPassword(); err != nil {
				log.Printf("Nextcloud#%d: revoke app password: %v", s.id, err)
			}
		}
	}
	cfg.Username, cfg.AppPassword = "", ""
	if err := c.EncodeConfig(cfg); err != nil {
		return err
	}
	if err := s.conns.Update(&c); err != nil {
		return err
	}
	s.Reconfigure()
	return nil
}

// Reconfigure is called after the connection row changed: drop the cached
// client so the next call rebuilds it, and re-arm the auto-sync timer.
func (s *NextcloudService) Reconfigure() {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	s.autoSync.TriggerReset()
}

// ConnectionID implements AlbumSource.
func (s *NextcloudService) ConnectionID() uint { return s.id }

// Source implements AlbumSource.
func (s *NextcloudService) Source() string { return model.SourceNextcloudMemories }

func (s *NextcloudService) scope() sourceScope {
	return sourceScope{Source: model.SourceNextcloudMemories, ConnectionID: s.id}
}

func (s *NextcloudService) config() (model.Connection, model.NextcloudConfig, error) {
	c, err := s.conns.Get(s.id)
	if err != nil {
		return c, model.NextcloudConfig{}, err
	}
	return c, c.NextcloudConfig(), nil
}

func (s *NextcloudService) isAutoSyncConfigured() bool {
	c, cfg, err := s.config()
	if err != nil || !c.Enabled || cfg.URL == "" || cfg.Username == "" || cfg.AppPassword == "" {
		return false
	}
	_, enabled := countAlbums(s.db, s.scope())
	return enabled > 0
}

func (s *NextcloudService) getAutoSyncConfig() (bool, time.Duration) {
	_, cfg, err := s.config()
	if err != nil {
		return false, time.Hour
	}
	return autoSyncConfig(cfg.AutoSyncEnabled, cfg.AutoSyncIntervalMinutes)
}

// getClient returns the current client, rebuilt when the URL or credentials
// changed. The returned pointer stays usable if s.client is replaced later.
func (s *NextcloudService) getClient() (*nextcloud.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, cfg, err := s.config()
	if err != nil {
		return nil, err
	}
	if cfg.URL == "" || cfg.Username == "" || cfg.AppPassword == "" {
		return nil, errors.New("nextcloud credentials not configured")
	}
	if s.client == nil || s.client.BaseURL != cfg.URL || s.client.Username != cfg.Username ||
		s.client.AppPassword != cfg.AppPassword {
		s.client = nextcloud.NewClient(cfg.URL, cfg.Username, cfg.AppPassword)
	}
	return s.client, nil
}

// TestConnection builds a fresh client and checks the credentials and that
// Memories answers.
func (s *NextcloudService) TestConnection() error {
	s.mu.Lock()
	s.client = nil
	s.mu.Unlock()
	client, err := s.getClient()
	if err != nil {
		return err
	}
	return client.TestConnection()
}

// NextcloudRemoteAlbum is a Memories album as the album picker shows it.
type NextcloudRemoteAlbum struct {
	ID           string `json:"id"` // cluster id ("owner/name"), the album's external id
	Name         string `json:"name"`
	Owner        string `json:"owner"`
	Count        int    `json:"count"`
	SharedWithMe bool   `json:"shared_with_me"`
}

// ListAlbums returns the albums the user owns and those shared with them.
func (s *NextcloudService) ListAlbums() ([]NextcloudRemoteAlbum, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	list, err := client.ListAlbums()
	if err != nil {
		return nil, err
	}
	out := make([]NextcloudRemoteAlbum, 0, len(list))
	for _, a := range list {
		out = append(out, NextcloudRemoteAlbum{
			ID: string(a.ClusterID), Name: a.Name, Owner: a.User, Count: a.Count,
			SharedWithMe: a.User != "" && a.User != client.Username,
		})
	}
	return out, nil
}

// ListRemoteAlbums implements AlbumSource (best-effort name refresh).
func (s *NextcloudService) ListRemoteAlbums() ([]RemoteAlbum, error) {
	list, err := s.ListAlbums()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteAlbum, 0, len(list))
	for _, a := range list {
		out = append(out, RemoteAlbum{ExternalID: a.ID, Name: a.Name})
	}
	return out, nil
}

// SetSyncAlbums defines the albums to sync: the given real albums (cluster
// ids) plus the picked virtual collections. Nothing is imported here; the
// next manual or automatic sync does that.
func (s *NextcloudService) SetSyncAlbums(realIDs []string, favorites, all, memories bool) error {
	nameByID := map[string]string{}
	if list, err := s.ListAlbums(); err == nil {
		for _, a := range list {
			nameByID[a.ID] = a.Name
		}
	}
	err := setSyncAlbumsWithVirtual(s.db, s.scope(), realIDs, nameByID, VirtualSelection{
		All: all, Favorites: favorites, Memories: memories,
	}, "On This Day")
	if err != nil {
		return err
	}
	// Re-arm the scheduler, which may sit on its "not configured" delay
	// (see ImmichService.SetSyncAlbums).
	s.autoSync.TriggerReset()
	return nil
}

// fetchPhotos returns the photos of one album, real or virtual.
func (s *NextcloudService) fetchPhotos(client *nextcloud.Client, album model.Album) ([]nextcloud.Photo, error) {
	if album.Kind == model.AlbumKindVirtual {
		switch album.ExternalID {
		case model.VirtualAll:
			return client.GetPhotos(nextcloud.Filter{})
		case model.VirtualFavorites:
			return client.GetPhotos(nextcloud.Filter{Favorites: true})
		case model.VirtualMemories:
			_, cfg, _ := s.config()
			return client.GetOnThisDay(cfg.MemoryMode == ImmichMemoryModeLatest, time.Now())
		default:
			return nil, fmt.Errorf("unknown virtual album: %q", album.ExternalID)
		}
	}
	return client.GetPhotos(nextcloud.Filter{Album: album.ExternalID})
}

// FetchAlbumAssets implements AlbumSource: the album's servable images as
// source-agnostic assets keyed by Nextcloud file id.
func (s *NextcloudService) FetchAlbumAssets(album model.Album) ([]RemoteAsset, error) {
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	photos, err := s.fetchPhotos(client, album)
	if err != nil {
		return nil, err
	}
	out := make([]RemoteAsset, 0, len(photos))
	seen := make(map[int64]bool, len(photos))
	for _, p := range photos {
		if !nextcloud.IsServableImage(p) || seen[p.FileID] {
			continue
		}
		seen[p.FileID] = true
		out = append(out, RemoteAsset{
			ExternalID:   strconv.FormatInt(p.FileID, 10),
			FilePath:     p.Basename,
			Width:        p.W,
			Height:       p.H,
			Orientation:  determineOrientation(p.W, p.H, ""),
			PhotoTakenAt: p.TakenAt(),
		})
	}
	return out, nil
}

// ImportPhotos syncs every sync-enabled album through the shared engine.
// Incremental (upsert + prune), never a clear first; see
// ImmichService.resyncInternal for why.
func (s *NextcloudService) ImportPhotos() error {
	if _, err := s.getClient(); err != nil {
		return err
	}
	_, err := SyncAlbumSource(s.db, s)
	return err
}

// ClearAndResync starts a sync in the background (the name is the
// PhotoSyncBackend interface's; nothing is cleared).
func (s *NextcloudService) ClearAndResync() error {
	s.autoSync.SyncNowAsync()
	return nil
}

// IsSyncing reports whether a sync is running.
func (s *NextcloudService) IsSyncing() bool { return s.autoSync.IsRunning() }

// LastSyncError reports the most recent sync run's failure ("" on success).
func (s *NextcloudService) LastSyncError() string { return s.autoSync.LastError() }

// ClearPhotos deletes this connection's photos from the database.
func (s *NextcloudService) ClearPhotos() error { return clearSourcePhotos(s.db, s.scope()) }

// GetPhotoCount returns the number of this connection's photos.
func (s *NextcloudService) GetPhotoCount() (int64, error) { return sourcePhotoCount(s.db, s.scope()) }

func parseFileID(externalID string) (int64, error) {
	id, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid nextcloud file id %q", externalID)
	}
	return id, nil
}

// GetThumbnail returns a small preview of a file for the gallery.
func (s *NextcloudService) GetThumbnail(externalID string) ([]byte, error) {
	id, err := parseFileID(externalID)
	if err != nil {
		return nil, err
	}
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	return client.GetPreview(id, 256)
}

// DownloadPhoto returns the full image as an upright JPEG: Memories'
// decodable rendition (HEIC already converted server-side) run through
// ImageMagick for EXIF orientation. A large preview stands in when the
// download or the conversion fails.
func (s *NextcloudService) DownloadPhoto(externalID string) ([]byte, error) {
	id, err := parseFileID(externalID)
	if err != nil {
		return nil, err
	}
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	data, err := client.DownloadDecodable(id)
	if err == nil {
		out, cerr := convertToJPEG(data)
		if cerr == nil {
			return out, nil
		}
		err = cerr
	}
	log.Printf("Nextcloud file %d: %v, falling back to preview", id, err)
	preview, perr := client.GetPreview(id, 2048)
	if perr != nil {
		return nil, fmt.Errorf("download failed: %w; preview fallback also failed: %v", err, perr)
	}
	// Previews may be WebP, which Go cannot decode; convert when possible.
	if out, cerr := convertToJPEG(preview); cerr == nil {
		return out, nil
	}
	return preview, nil
}
