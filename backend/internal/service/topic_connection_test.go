package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

func newTopicManager(t *testing.T, db *gorm.DB) *ConnectionManager {
	t.Helper()
	m := NewConnectionManager(db, nil, nil, nil)
	m.Start()
	return m
}

func topicOf(t *testing.T, m *ConnectionManager, c model.Connection) *TopicSource {
	t.Helper()
	svc, err := m.Topic(c.ID)
	require.NoError(t, err)
	return svc
}

// Two Unsplash keys keep their own topic sets, even for the same topic text:
// editing one connection's topics never touches the other's albums.
func TestTopicSetSyncTopicsIsScopedToTheConnection(t *testing.T) {
	db := setupGoogleDB(t)
	m := newTopicManager(t, db)
	a := mkConnection(t, db, model.SourceUnsplash, "Home", model.TopicConfig{APIKey: "ka"})
	b := mkConnection(t, db, model.SourceUnsplash, "Work", model.TopicConfig{APIKey: "kb"})
	sa, sb := topicOf(t, m, a), topicOf(t, m, b)

	require.NoError(t, sa.SetSyncTopics([]string{"cats", " dogs ", "cats"}))
	require.NoError(t, sb.SetSyncTopics([]string{"cats"}))

	ta, err := sa.Topics()
	require.NoError(t, err)
	assert.Equal(t, []string{"cats", "dogs"}, ta)
	tb, err := sb.Topics()
	require.NoError(t, err)
	assert.Equal(t, []string{"cats"}, tb)

	var albums []model.Album
	require.NoError(t, db.Where("source = ?", model.SourceUnsplash).Find(&albums).Error)
	assert.Len(t, albums, 3)
	for _, al := range albums {
		require.NotNil(t, al.ConnectionID, "topic album %q must belong to a connection", al.ExternalID)
		assert.True(t, al.SyncEnabled)
		assert.Equal(t, model.AlbumKindReal, al.Kind)
	}

	// Dropping a topic from A leaves B's identical topic alone.
	require.NoError(t, sa.SetSyncTopics([]string{"dogs"}))
	ta, _ = sa.Topics()
	assert.Equal(t, []string{"dogs"}, ta)
	tb, _ = sb.Topics()
	assert.Equal(t, []string{"cats"}, tb)
}

// TestConnection needs the connection's own key and hands it, with
// randomize off, to the search; a sync passes the connection's randomize
// setting.
func TestTopicSearchUsesTheConnectionsKey(t *testing.T) {
	db := setupGoogleDB(t)
	m := newTopicManager(t, db)
	c := mkConnection(t, db, model.SourcePexels, "Pexels", model.TopicConfig{})
	svc := topicOf(t, m, c)

	type call struct {
		key, query string
		randomize  bool
		limit      int
	}
	var calls []call
	svc.search = func(apiKey string, randomize bool, query string, limit int) ([]RemoteAsset, error) {
		calls = append(calls, call{apiKey, query, randomize, limit})
		return []RemoteAsset{{ExternalID: "1", FilePath: "https://x/1"}}, nil
	}

	err := svc.TestConnection()
	require.Error(t, err, "no key stored yet")
	assert.Contains(t, err.Error(), "api key not configured")
	assert.Empty(t, calls)

	require.NoError(t, c.EncodeConfig(model.TopicConfig{APIKey: "pk", RandomizeResults: true}))
	require.NoError(t, m.Store().Update(&c))
	require.NoError(t, svc.TestConnection())
	require.Len(t, calls, 1)
	assert.Equal(t, call{"pk", "landscape", false, 1}, calls[0])

	assets, err := svc.FetchAlbumAssets(model.Album{ExternalID: "mountains"})
	require.NoError(t, err)
	assert.Len(t, assets, 1)
	require.Len(t, calls, 2)
	assert.Equal(t, call{"pk", "mountains", true, topicSyncLimit}, calls[1])
}

// Auto-sync runs only for an enabled connection with a key and at least one
// topic.
func TestTopicAutoSyncConfigured(t *testing.T) {
	db := setupGoogleDB(t)
	m := newTopicManager(t, db)
	c := mkConnection(t, db, model.SourceUnsplash, "Unsplash", model.TopicConfig{})
	svc := topicOf(t, m, c)

	assert.False(t, svc.isAutoSyncConfigured(), "no key")

	require.NoError(t, c.EncodeConfig(model.TopicConfig{APIKey: "k", AutoSyncEnabled: true, AutoSyncIntervalMinutes: 30}))
	require.NoError(t, m.Store().Update(&c))
	assert.False(t, svc.isAutoSyncConfigured(), "no topics")

	require.NoError(t, svc.SetSyncTopics([]string{"sea"}))
	assert.True(t, svc.isAutoSyncConfigured())
	enabled, every := svc.getAutoSyncConfig()
	assert.True(t, enabled)
	assert.Equal(t, "30m0s", every.String())

	c.Enabled = false
	require.NoError(t, m.Store().Update(&c))
	assert.False(t, svc.isAutoSyncConfigured(), "disabled connection")
}

// A topic some frame is limited to cannot be removed from the set: the
// frame would widen to the connection's other topics. Other topics come and
// go as before, and once the frame is moved the topic goes too.
func TestTopicRemovalRefusedWhileFramesUseIt(t *testing.T) {
	db := setupGoogleDB(t)
	m := newTopicManager(t, db)
	c := mkConnection(t, db, model.SourceUnsplash, "Unsplash", model.TopicConfig{APIKey: "k"})
	svc := topicOf(t, m, c)
	require.NoError(t, svc.SetSyncTopics([]string{"sea", "sky"}))
	var sea model.Album
	require.NoError(t, db.Where("external_id = ?", "sea").First(&sea).Error)
	frame := model.Device{Name: "Study", Host: "10.0.0.2", Source: model.SourceUnsplash, ImageConnectionID: &c.ID}
	require.NoError(t, db.Create(&frame).Error)
	require.NoError(t, db.Create(&model.DeviceAlbumMapping{DeviceID: frame.ID, AlbumID: sea.ID}).Error)

	err := svc.SetSyncTopics([]string{"sky"})
	var inUse *InUseError
	require.ErrorAs(t, err, &inUse)
	assert.Contains(t, err.Error(), `the topic "sea" is used by 1 frame: Study`)
	topics, err := svc.Topics()
	require.NoError(t, err)
	assert.Equal(t, []string{"sea", "sky"}, topics, "the refused change is rolled back")

	require.NoError(t, svc.SetSyncTopics([]string{"sea"}), "dropping an unbound topic is fine")
	err = m.Delete(c.ID)
	require.ErrorAs(t, err, &inUse, "the connection itself is in use too")

	// Switched to another source, the frame only remembers "sea": the topic
	// can go, and the memory goes with it.
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", frame.ID).
		Updates(map[string]any{"source": model.SourceGallery, "image_connection_id": nil}).Error)
	require.NoError(t, svc.SetSyncTopics([]string{"sky"}))
	topics, _ = svc.Topics()
	assert.Equal(t, []string{"sky"}, topics)
	assert.Empty(t, DeviceAlbumIDs(db, frame.ID, model.SourceUnsplash))
	require.NoError(t, m.Delete(c.ID), "and so can the connection")
}

// The generic sync surface resolves a topic connection, and deleting the
// connection stops and forgets its service.
func TestTopicBackendAndDelete(t *testing.T) {
	db := setupGoogleDB(t)
	m := newTopicManager(t, db)
	c := mkConnection(t, db, model.SourceUnsplash, "Unsplash", model.TopicConfig{APIKey: "k"})
	svc := topicOf(t, m, c)
	require.NoError(t, svc.SetSyncTopics([]string{"sea"}))

	b, err := m.Backend(c.ID)
	require.NoError(t, err)
	assert.Same(t, svc, b)
	assert.False(t, b.IsSyncing())
	n, err := b.GetPhotoCount()
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = m.Immich(c.ID)
	require.Error(t, err, "a topic connection is not an Immich one")

	require.NoError(t, m.Delete(c.ID))
	_, err = m.Topic(c.ID)
	assert.True(t, errors.Is(err, ErrConnectionNotFound), "got %v", err)
}
