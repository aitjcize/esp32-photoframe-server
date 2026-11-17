package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

// ErrConnectionNotFound is returned for an unknown connection id.
var ErrConnectionNotFound = errors.New("connection not found")

// ConnectionStore is the persistence side of connections: the rows only.
// Services read their configuration through it on every use, the way they
// used to read the settings table, so an edit takes effect on the next call.
type ConnectionStore struct {
	db *gorm.DB
}

// NewConnectionStore wraps db.
func NewConnectionStore(db *gorm.DB) *ConnectionStore { return &ConnectionStore{db: db} }

// Get returns one connection.
func (st *ConnectionStore) Get(id uint) (model.Connection, error) {
	var c model.Connection
	if err := st.db.First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c, ErrConnectionNotFound
		}
		return c, err
	}
	return c, nil
}

// List returns the connections of one source type, or all of them when source
// is empty, oldest first so the migrated connection stays on top.
func (st *ConnectionStore) List(source string) ([]model.Connection, error) {
	q := st.db.Model(&model.Connection{}).Order("id")
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var out []model.Connection
	err := q.Find(&out).Error
	return out, err
}

// Create inserts c (its ID is filled in).
func (st *ConnectionStore) Create(c *model.Connection) error {
	if !model.IsConnectionSource(c.Source) {
		return fmt.Errorf("source %q is not configured through connections", c.Source)
	}
	if c.Config == "" {
		c.Config = "{}"
	}
	if c.State == "" {
		c.State = "{}"
	}
	now := time.Now()
	c.CreatedAt, c.UpdatedAt = now, now
	return st.db.Create(c).Error
}

// Update writes the user-editable columns: name, enabled, config. State is
// left alone so a session the service saved meanwhile is not clobbered.
func (st *ConnectionStore) Update(c *model.Connection) error {
	return st.db.Model(&model.Connection{}).Where("id = ?", c.ID).Updates(map[string]any{
		"name":       c.Name,
		"enabled":    c.Enabled,
		"config":     c.Config,
		"updated_at": time.Now(),
	}).Error
}

// SetState writes just the state column.
func (st *ConnectionStore) SetState(id uint, state string) error {
	return st.db.Model(&model.Connection{}).Where("id = ?", id).Update("state", state).Error
}

// Delete removes the row; its albums and images follow through ON DELETE
// CASCADE (migration 000041).
func (st *ConnectionStore) Delete(id uint) error {
	res := st.db.Delete(&model.Connection{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

// ConnectionManager owns one service per connection row: an ImmichService or
// SynologyService with its own client cache, login state and auto-sync
// scheduler, a GooglePhotosService with its own OAuth token, or a TopicSource
// (Unsplash, Pexels) with its own API key and topics. It builds them at
// startup, on create, and stops them on delete. Handlers and the serving
// plugins look services up here by connection id, never by source type.
type ConnectionManager struct {
	db    *gorm.DB
	store *ConnectionStore
	// The global OAuth client with the Photos scopes, and the same client
	// with the Calendar scopes.
	googleCfg   googlephotos.ConfigProvider
	calendarCfg googlephotos.ConfigProvider
	picker      *PickerService

	mu        sync.Mutex
	immich    map[uint]*ImmichService
	synology  map[uint]*SynologyService
	google    map[uint]*GooglePhotosService
	calendars map[uint]*GoogleCalendarService
	topics    map[uint]*TopicSource

	// Pending Google OAuth flows by state nonce: the callback carries only
	// the nonce, which names the connection the code belongs to.
	oauth map[string]pendingOAuth
}

type pendingOAuth struct {
	connectionID uint
	redirectURL  string
	expires      time.Time
}

// NewConnectionManager builds an empty manager; Start loads the rows.
// googleCfg supplies the global Google OAuth client id and secret; picker
// is the shared Google Photos Picker flow.
func NewConnectionManager(db *gorm.DB, googleCfg, calendarCfg googlephotos.ConfigProvider, picker *PickerService) *ConnectionManager {
	return &ConnectionManager{
		db:          db,
		store:       NewConnectionStore(db),
		googleCfg:   googleCfg,
		calendarCfg: calendarCfg,
		picker:      picker,
		immich:      map[uint]*ImmichService{},
		synology:    map[uint]*SynologyService{},
		google:      map[uint]*GooglePhotosService{},
		calendars:   map[uint]*GoogleCalendarService{},
		topics:      map[uint]*TopicSource{},
		oauth:       map[string]pendingOAuth{},
	}
}

// Store exposes the row store for handlers.
func (m *ConnectionManager) Store() *ConnectionStore { return m.store }

// Start builds a service for every stored connection and starts their
// auto-sync loops.
func (m *ConnectionManager) Start() {
	rows, err := m.store.List("")
	if err != nil {
		log.Printf("connections: load: %v", err)
		return
	}
	for _, c := range rows {
		if _, err := m.ensure(c); err != nil {
			log.Printf("connections: %s #%d (%s): %v", c.Source, c.ID, c.Name, err)
		}
	}
}

// ensure returns the service for c, creating and starting it on first use.
func (m *ConnectionManager) ensure(c model.Connection) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch c.Source {
	case model.SourceImmich:
		if s, ok := m.immich[c.ID]; ok {
			return s, nil
		}
		s := NewImmichService(m.db, m.store, c.ID)
		s.StartAutoSync()
		m.immich[c.ID] = s
		return s, nil
	case model.SourceSynologyPhotos:
		if s, ok := m.synology[c.ID]; ok {
			return s, nil
		}
		s := NewSynologyService(m.db, m.store, c.ID)
		s.StartAutoSync()
		m.synology[c.ID] = s
		return s, nil
	case model.SourceGooglePhotos:
		if s, ok := m.google[c.ID]; ok {
			return s, nil
		}
		s := NewGooglePhotosService(m.db, m.store, c.ID, m.googleCfg, m.picker)
		m.google[c.ID] = s
		return s, nil
	case model.SourceGoogleCalendar:
		if s, ok := m.calendars[c.ID]; ok {
			return s, nil
		}
		s := NewGoogleCalendarService(m.store, c.ID, m.calendarCfg)
		m.calendars[c.ID] = s
		return s, nil
	case model.SourceUnsplash, model.SourcePexels:
		if s, ok := m.topics[c.ID]; ok {
			return s, nil
		}
		s, err := NewTopicSource(m.db, m.store, c.ID, c.Source)
		if err != nil {
			return nil, err
		}
		s.StartAutoSync()
		m.topics[c.ID] = s
		return s, nil
	}
	return nil, fmt.Errorf("source %q has no connection service", c.Source)
}

// Create stores a new connection and starts its service.
func (m *ConnectionManager) Create(c *model.Connection) error {
	if err := m.store.Create(c); err != nil {
		return err
	}
	_, err := m.ensure(*c)
	return err
}

// Updated tells the connection's service that its row changed: the client is
// rebuilt on next use and the auto-sync timer is re-armed.
func (m *ConnectionManager) Updated(id uint) {
	m.mu.Lock()
	im, sy, tp := m.immich[id], m.synology[id], m.topics[id]
	m.mu.Unlock()
	if im != nil {
		im.Reconfigure()
	}
	if sy != nil {
		sy.Reconfigure()
	}
	if tp != nil {
		tp.Reconfigure()
	}
}

// Delete stops the connection's service and removes the row, which cascades
// to its albums and images (and with the albums, every frame's remembered
// selection of them); cached thumbnails of those images are swept. A
// connection some frame draws from (as its image source, through one of its
// albums while on "any connection", or as its calendar account) is refused
// with an InUseError: the frame would otherwise move to a wider pool without
// anyone choosing it.
func (m *ConnectionManager) Delete(id uint) error {
	row, err := m.store.Get(id)
	if err != nil {
		return err
	}
	if users, err := FramesUsingConnection(m.db, row); err != nil {
		return err
	} else if len(users) > 0 {
		return &InUseError{What: fmt.Sprintf("the connection %q", row.Name), Devices: users}
	}
	m.mu.Lock()
	if s, ok := m.immich[id]; ok {
		s.Stop()
		delete(m.immich, id)
	}
	if s, ok := m.synology[id]; ok {
		s.Stop()
		delete(m.synology, id)
	}
	if s, ok := m.topics[id]; ok {
		s.Stop()
		delete(m.topics, id)
	}
	g, isGoogle := m.google[id]
	if isGoogle {
		delete(m.google, id)
	}
	if s, ok := m.calendars[id]; ok {
		s.Stop()
		delete(m.calendars, id)
	}
	m.mu.Unlock()
	// Outside the lock: stopping a Google account waits for its running
	// imports to notice, and nothing else needs the manager meanwhile.
	if isGoogle {
		g.Stop()
	}
	// Frames that remember this account (calendar switched off) lose the
	// binding with the row (ON DELETE SET NULL); the calendar id, which
	// another account may not be able to read, goes with it.
	if row.Source == model.SourceGoogleCalendar {
		if err := m.db.Model(&model.Device{}).Where("calendar_connection_id = ?", id).
			Updates(map[string]any{"calendar_connection_id": nil, "calendar_id": ""}).Error; err != nil {
			return err
		}
	}
	// Google photos are local files; take them off disk before the cascade
	// forgets where they are.
	if isGoogle {
		if err := g.ClearPhotos(); err != nil {
			return err
		}
	}
	if err := m.store.Delete(id); err != nil {
		return err
	}
	gcOrphanThumbnails(m.db)
	return nil
}

// Immich returns the service for an Immich connection.
func (m *ConnectionManager) Immich(id uint) (*ImmichService, error) {
	m.mu.Lock()
	s, ok := m.immich[id]
	m.mu.Unlock()
	if ok {
		return s, nil
	}
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Source != model.SourceImmich {
		return nil, fmt.Errorf("connection %d is %s, not immich", id, c.Source)
	}
	v, err := m.ensure(c)
	if err != nil {
		return nil, err
	}
	return v.(*ImmichService), nil
}

// Synology returns the service for a Synology Photos connection.
func (m *ConnectionManager) Synology(id uint) (*SynologyService, error) {
	m.mu.Lock()
	s, ok := m.synology[id]
	m.mu.Unlock()
	if ok {
		return s, nil
	}
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Source != model.SourceSynologyPhotos {
		return nil, fmt.Errorf("connection %d is %s, not synology_photos", id, c.Source)
	}
	v, err := m.ensure(c)
	if err != nil {
		return nil, err
	}
	return v.(*SynologyService), nil
}

// Google returns the service for a Google Photos connection.
func (m *ConnectionManager) Google(id uint) (*GooglePhotosService, error) {
	m.mu.Lock()
	s, ok := m.google[id]
	m.mu.Unlock()
	if ok {
		return s, nil
	}
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Source != model.SourceGooglePhotos {
		return nil, fmt.Errorf("connection %d is %s, not google_photos", id, c.Source)
	}
	v, err := m.ensure(c)
	if err != nil {
		return nil, err
	}
	return v.(*GooglePhotosService), nil
}

// Calendar returns the service for a Google Calendar connection.
func (m *ConnectionManager) Calendar(id uint) (*GoogleCalendarService, error) {
	m.mu.Lock()
	s, ok := m.calendars[id]
	m.mu.Unlock()
	if ok {
		return s, nil
	}
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Source != model.SourceGoogleCalendar {
		return nil, fmt.Errorf("connection %d is %s, not google_calendar", id, c.Source)
	}
	v, err := m.ensure(c)
	if err != nil {
		return nil, err
	}
	return v.(*GoogleCalendarService), nil
}

// CalendarForDevice resolves the Google account whose calendar a frame shows:
// the one it is bound to, or else the first connected account, which keeps
// frames set up before accounts were plural working.
func (m *ConnectionManager) CalendarForDevice(d model.Device) (*GoogleCalendarService, error) {
	if d.CalendarConnectionID != nil {
		return m.Calendar(*d.CalendarConnectionID)
	}
	rows, err := m.store.List(model.SourceGoogleCalendar)
	if err != nil {
		return nil, err
	}
	for _, c := range rows {
		if !c.Enabled || !c.GoogleState().Connected() {
			continue
		}
		return m.Calendar(c.ID)
	}
	return nil, ErrCalendarNotConnected
}

// googleAccount returns the OAuth side of a Google Photos or Google Calendar
// connection, whichever it is.
func (m *ConnectionManager) googleAccount(id uint) (*googleAccount, error) {
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	switch c.Source {
	case model.SourceGooglePhotos:
		s, err := m.Google(id)
		if err != nil {
			return nil, err
		}
		return s.googleAccount, nil
	case model.SourceGoogleCalendar:
		s, err := m.Calendar(id)
		if err != nil {
			return nil, err
		}
		return s.googleAccount, nil
	}
	return nil, fmt.Errorf("connection %d is %s, not a Google account", id, c.Source)
}

// GoogleLogout forgets a Google Photos or Calendar account's token; the
// connection and (for Photos) its imported photos stay.
func (m *ConnectionManager) GoogleLogout(id uint) error {
	a, err := m.googleAccount(id)
	if err != nil {
		return err
	}
	return a.Logout()
}

// Topic returns the service for an Unsplash or Pexels connection.
func (m *ConnectionManager) Topic(id uint) (*TopicSource, error) {
	m.mu.Lock()
	s, ok := m.topics[id]
	m.mu.Unlock()
	if ok {
		return s, nil
	}
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	if !model.IsTopicSource(c.Source) {
		return nil, fmt.Errorf("connection %d is %s, not a topic source", id, c.Source)
	}
	v, err := m.ensure(c)
	if err != nil {
		return nil, err
	}
	return v.(*TopicSource), nil
}

// oauthStateTTL bounds how long a consent page may sit open before the
// callback is refused.
const oauthStateTTL = 15 * time.Minute

// BeginGoogleAuth starts the consent flow for a Google Photos or Google
// Calendar connection and returns the URL to send the browser to. redirectURL
// is the server's callback as the browser reaches it. The state parameter is a
// random nonce remembered here, so the callback can only complete a flow this
// server started.
func (m *ConnectionManager) BeginGoogleAuth(id uint, redirectURL string) (string, error) {
	svc, err := m.googleAccount(id)
	if err != nil {
		return "", err
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	url, err := svc.AuthURL(redirectURL, nonce)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	now := time.Now()
	for k, p := range m.oauth {
		if p.expires.Before(now) {
			delete(m.oauth, k)
		}
	}
	m.oauth[nonce] = pendingOAuth{connectionID: id, redirectURL: redirectURL, expires: now.Add(oauthStateTTL)}
	m.mu.Unlock()
	return url, nil
}

// ErrUnknownOAuthState is returned for a callback whose state this server did
// not issue or that has expired.
var ErrUnknownOAuthState = errors.New("unknown or expired OAuth state")

// CompleteGoogleAuth exchanges the callback's code for the connection the
// state names and returns that connection's id.
func (m *ConnectionManager) CompleteGoogleAuth(state, code string) (uint, error) {
	m.mu.Lock()
	p, ok := m.oauth[state]
	delete(m.oauth, state)
	m.mu.Unlock()
	if !ok || p.expires.Before(time.Now()) {
		return 0, ErrUnknownOAuthState
	}
	svc, err := m.googleAccount(p.connectionID)
	if err != nil {
		return 0, err
	}
	if err := svc.Exchange(p.redirectURL, code); err != nil {
		return 0, err
	}
	return p.connectionID, nil
}

func randomNonce() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ImmichForImage resolves the Immich connection an image was synced from.
func (m *ConnectionManager) ImmichForImage(img model.Image) (*ImmichService, error) {
	if img.ConnectionID == nil {
		return nil, fmt.Errorf("image %d has no connection", img.ID)
	}
	return m.Immich(*img.ConnectionID)
}

// SynologyForImage resolves the Synology connection an image was synced from.
func (m *ConnectionManager) SynologyForImage(img model.Image) (*SynologyService, error) {
	if img.ConnectionID == nil {
		return nil, fmt.Errorf("image %d has no connection", img.ID)
	}
	return m.Synology(*img.ConnectionID)
}

// SyncBackend is the per-connection surface the generic sync endpoints use.
// Every connection service implements it (it matches handler.PhotoSyncBackend).
type SyncBackend interface {
	ClearAndResync() error
	IsSyncing() bool
	LastSyncError() string
	ClearPhotos() error
	GetPhotoCount() (int64, error)
}

// Backend returns the sync surface of any connection, whatever its type.
func (m *ConnectionManager) Backend(id uint) (SyncBackend, error) {
	c, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	switch c.Source {
	case model.SourceImmich:
		return m.Immich(id)
	case model.SourceSynologyPhotos:
		return m.Synology(id)
	case model.SourceGooglePhotos:
		return m.Google(id)
	case model.SourceUnsplash, model.SourcePexels:
		return m.Topic(id)
	case model.SourceGoogleCalendar:
		return nil, fmt.Errorf("a Google Calendar connection has no photos")
	}
	return nil, fmt.Errorf("source %q has no sync backend", c.Source)
}

// autoSyncConfig reads the shared auto-sync pair out of a connection config
// with the 60-minute default the settings keys used to have.
func autoSyncConfig(enabled bool, minutes int) (bool, time.Duration) {
	if minutes <= 0 {
		minutes = 60
	}
	return enabled, time.Duration(minutes) * time.Minute
}
