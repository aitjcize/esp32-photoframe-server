package service

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
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
// scheduler. It builds them at startup, on create, and stops them on delete.
// Handlers and the serving plugins look services up here by connection id,
// never by source type.
type ConnectionManager struct {
	db    *gorm.DB
	store *ConnectionStore

	mu       sync.Mutex
	immich   map[uint]*ImmichService
	synology map[uint]*SynologyService
}

// NewConnectionManager builds an empty manager; Start loads the rows.
func NewConnectionManager(db *gorm.DB) *ConnectionManager {
	return &ConnectionManager{
		db:       db,
		store:    NewConnectionStore(db),
		immich:   map[uint]*ImmichService{},
		synology: map[uint]*SynologyService{},
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
	im, sy := m.immich[id], m.synology[id]
	m.mu.Unlock()
	if im != nil {
		im.Reconfigure()
	}
	if sy != nil {
		sy.Reconfigure()
	}
}

// Delete stops the connection's service and removes the row, which cascades
// to its albums and images; cached thumbnails of those images are swept.
func (m *ConnectionManager) Delete(id uint) error {
	m.mu.Lock()
	if s, ok := m.immich[id]; ok {
		s.Stop()
		delete(m.immich, id)
	}
	if s, ok := m.synology[id]; ok {
		s.Stop()
		delete(m.synology, id)
	}
	m.mu.Unlock()
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
// Both connection services implement it (it matches handler.PhotoSyncBackend).
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
