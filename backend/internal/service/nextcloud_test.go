package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// fakeMemories serves the slice of the Memories API the sync uses: one album
// "alice/Trip" with two images and a video, and favorites holding one of the
// album's images plus another.
func fakeMemories(t *testing.T) *httptest.Server {
	t.Helper()
	const api = "/index.php/apps/memories/api"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		switch {
		case r.URL.Path == api+"/clusters/albums":
			_, _ = w.Write([]byte(`[
				{"album_id":1,"cluster_id":"alice/Trip","name":"Trip","user":"alice","count":3},
				{"album_id":2,"cluster_id":"bob/Party","name":"Party","user":"bob","count":1}
			]`))
		case r.URL.Path == api+"/days":
			_, _ = w.Write([]byte(`[{"dayid":20000,"count":3}]`))
		case strings.HasPrefix(r.URL.Path, api+"/days/"):
			var photos []map[string]any
			switch {
			case q.Get("albums") == "alice/Trip":
				photos = []map[string]any{
					{"fileid": 11, "basename": "a.jpg", "mimetype": "image/jpeg", "dayid": 20000, "w": 4000, "h": 3000},
					{"fileid": 12, "basename": "b.heic", "mimetype": "image/heic", "dayid": 20000, "w": 3000, "h": 4000},
					{"fileid": 13, "basename": "c.mp4", "mimetype": "video/mp4", "dayid": 20000, "isvideo": 1},
				}
			case q.Get("fav") == "1":
				photos = []map[string]any{
					{"fileid": 11, "basename": "a.jpg", "mimetype": "image/jpeg", "dayid": 20000, "w": 4000, "h": 3000},
					{"fileid": 14, "basename": "d.png", "mimetype": "image/png", "dayid": 20000, "w": 100, "h": 100},
				}
			default:
				t.Errorf("unexpected day query %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(photos)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func nextcloudTestService(t *testing.T, url string) (*NextcloudService, model.Connection) {
	t.Helper()
	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Connection{}))
	c := mkConnection(t, db, model.SourceNextcloudMemories, "Nextcloud", model.NextcloudConfig{
		URL: url, Username: "alice", AppPassword: "secret",
	})
	return NewNextcloudService(db, NewConnectionStore(db), c.ID), c
}

func TestNextcloudListAlbumsMarksSharedWithMe(t *testing.T) {
	srv := fakeMemories(t)
	defer srv.Close()
	svc, _ := nextcloudTestService(t, srv.URL)

	albums, err := svc.ListAlbums()
	require.NoError(t, err)
	require.Len(t, albums, 2)
	assert.Equal(t, "alice/Trip", albums[0].ID)
	assert.False(t, albums[0].SharedWithMe)
	assert.True(t, albums[1].SharedWithMe, "bob's album is shared with alice")
}

// An album plus the Favorites collection sync into one deduplicated pool:
// the video is skipped, and the photo in both keeps one image row with two
// memberships. Deselecting the album drops only what Favorites doesn't hold.
func TestNextcloudSyncAlbumAndFavorites(t *testing.T) {
	srv := fakeMemories(t)
	defer srv.Close()
	svc, c := nextcloudTestService(t, srv.URL)
	db := svc.db

	require.NoError(t, svc.SetSyncAlbums([]string{"alice/Trip"}, true, false, false))
	var albums []model.Album
	require.NoError(t, db.Where("connection_id = ?", c.ID).Order("external_id").Find(&albums).Error)
	require.Len(t, albums, 2)
	assert.Equal(t, model.VirtualFavorites, albums[0].ExternalID)
	assert.Equal(t, "Trip", albums[1].Name, "the album is named from the listing")

	require.NoError(t, svc.ImportPhotos())

	var imgs []model.Image
	require.NoError(t, db.Where("source = ?", model.SourceNextcloudMemories).Order("external_id").Find(&imgs).Error)
	ids := []string{}
	for _, im := range imgs {
		ids = append(ids, im.ExternalID)
		assert.Equal(t, c.ID, *im.ConnectionID)
		assert.NotNil(t, im.PhotoTakenAt, "dated from the day id")
	}
	assert.Equal(t, []string{"11", "12", "14"}, ids, "video skipped, shared photo deduplicated")
	assert.Equal(t, "portrait", imgs[1].Orientation)

	var mems int64
	db.Model(&model.ImageAlbumMembership{}).Where("image_id = ?", imgs[0].ID).Count(&mems)
	assert.Equal(t, int64(2), mems, "photo 11 is in the album and in favorites")

	require.NoError(t, svc.SetSyncAlbums(nil, true, false, false))
	ids = ids[:0]
	db.Model(&model.Image{}).Where("source = ?", model.SourceNextcloudMemories).Order("external_id").Pluck("external_id", &ids)
	assert.Equal(t, []string{"11", "14"}, ids, "only the favorites remain")
}

// The login flow stores the user name and app password Nextcloud hands over,
// and the connection then counts as connected; logging out revokes and
// forgets them.
func TestNextcloudLoginFlowStoresCredentials(t *testing.T) {
	var polls atomic.Int32
	var revoked atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/index.php/login/v2":
			_, _ = w.Write([]byte(`{"poll":{"token":"tok","endpoint":"x"},"login":"https://nc/login/v2/flow/1"}`))
		case "/index.php/login/v2/poll":
			if polls.Add(1) < 3 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"server":"https://nc","loginName":"alice","appPassword":"generated"}`))
		case "/ocs/v2.php/core/apppassword":
			if u, p, _ := r.BasicAuth(); u == "alice" && p == "generated" && r.Method == "DELETE" {
				revoked.Store(true)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Connection{}))
	c := mkConnection(t, db, model.SourceNextcloudMemories, "Nextcloud", model.NextcloudConfig{URL: srv.URL})
	svc := NewNextcloudService(db, NewConnectionStore(db), c.ID)
	svc.loginHTTP = srv.Client()
	svc.loginPoll = 5 * time.Millisecond

	assert.Equal(t, NextcloudLoginIdle, svc.LoginStatus().State)
	url, err := svc.BeginLogin()
	require.NoError(t, err)
	assert.Equal(t, "https://nc/login/v2/flow/1", url)

	require.Eventually(t, func() bool { return svc.LoginStatus().State == NextcloudLoginDone },
		2*time.Second, 5*time.Millisecond, "status: %+v", svc.LoginStatus())
	stored, err := NewConnectionStore(db).Get(c.ID)
	require.NoError(t, err)
	cfg := stored.NextcloudConfig()
	assert.Equal(t, "alice", cfg.Username)
	assert.Equal(t, "generated", cfg.AppPassword)
	assert.Equal(t, srv.URL, cfg.URL, "the URL the user entered is kept")

	require.NoError(t, svc.Logout())
	assert.True(t, revoked.Load(), "the app password is revoked on the server")
	stored, _ = NewConnectionStore(db).Get(c.ID)
	assert.Empty(t, stored.NextcloudConfig().AppPassword)
	assert.Equal(t, srv.URL, stored.NextcloudConfig().URL)
}

func TestNextcloudLoginFlowExpires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.php/login/v2" {
			_, _ = w.Write([]byte(`{"poll":{"token":"tok"},"login":"https://nc/flow"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound) // never granted
	}))
	defer srv.Close()
	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Connection{}))
	c := mkConnection(t, db, model.SourceNextcloudMemories, "Nextcloud", model.NextcloudConfig{URL: srv.URL})
	svc := NewNextcloudService(db, NewConnectionStore(db), c.ID)
	svc.loginHTTP = srv.Client()
	svc.loginPoll = 5 * time.Millisecond
	svc.loginDeadline = 50 * time.Millisecond

	_, err := svc.BeginLogin()
	require.NoError(t, err)
	require.Eventually(t, func() bool { return svc.LoginStatus().State == NextcloudLoginFailed },
		2*time.Second, 5*time.Millisecond)
	assert.Contains(t, svc.LoginStatus().Error, "expired")
}

func TestNextcloudAutoSyncGate(t *testing.T) {
	srv := fakeMemories(t)
	defer srv.Close()
	svc, c := nextcloudTestService(t, srv.URL)

	assert.False(t, svc.isAutoSyncConfigured(), "nothing picked yet")
	mkConnAlbum(t, svc.db, c, model.VirtualMemories, model.AlbumKindVirtual, true)
	assert.True(t, svc.isAutoSyncConfigured())
}
