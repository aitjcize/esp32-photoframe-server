package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

// What a frame draws from is exactly what its owner chose, and stays chosen:
// these tests drive the device, connection and gallery endpoints the way
// the dashboard does.

var bindingsDBCounter atomic.Int64

type noGoogle struct{}

func (noGoogle) GetGoogleConfig() (googlephotos.Config, error) {
	return googlephotos.Config{ClientID: "id", ClientSecret: "secret"}, nil
}

type bindingsRig struct {
	db      *gorm.DB
	conns   *service.ConnectionManager
	devices *DeviceHandler
	connh   *ConnectionsHandler
	gallery *GalleryHandler
}

func newBindingsRig(t *testing.T) *bindingsRig {
	t.Helper()
	n := bindingsDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:bindings_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	serialiseTestDB(t, db)
	require.NoError(t, db.AutoMigrate(&model.Connection{}, &model.Device{}, &model.Album{}, &model.Image{},
		&model.ImageAlbumMembership{}, &model.DeviceAlbumMapping{}))
	conns := service.NewConnectionManager(db, noGoogle{}, noGoogle{}, nil)
	conns.Start()
	return &bindingsRig{
		db:      db,
		conns:   conns,
		devices: NewDeviceHandler(service.NewDeviceService(service.DeviceServiceDeps{DB: db, Connections: conns}), conns, db),
		connh:   NewConnectionsHandler(conns),
		gallery: NewGalleryHandler(db, conns, t.TempDir()),
	}
}

// call runs one handler as the router would: JSON body, :id param, query.
func call(t *testing.T, h echo.HandlerFunc, method, body string, id uint, query string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/?"+query, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprint(id))
	_ = h(c)
	return rec
}

func (r *bindingsRig) connection(t *testing.T, source, name string) model.Connection {
	t.Helper()
	c := model.Connection{Source: source, Name: name, Enabled: true, Config: "{}", State: "{}"}
	require.NoError(t, r.db.Create(&c).Error)
	return c
}

func (r *bindingsRig) album(t *testing.T, c *model.Connection, source, ext, name string) model.Album {
	t.Helper()
	a := model.Album{Source: source, ExternalID: ext, Name: name, Kind: model.AlbumKindReal, SyncEnabled: true}
	if c != nil {
		a.ConnectionID = &c.ID
	}
	require.NoError(t, r.db.Create(&a).Error)
	return a
}

// updateDevice is the dashboard's save: PUT /api/devices/:id with the
// source and connection, everything else kept.
func (r *bindingsRig) updateDevice(t *testing.T, d model.Device, source string, conn *model.Connection) {
	t.Helper()
	connID := "null"
	if conn != nil {
		connID = fmt.Sprint(conn.ID)
	}
	rec := call(t, r.devices.UpdateDevice, http.MethodPut,
		fmt.Sprintf(`{"name":%q,"host":%q,"source":%q,"image_connection_id":%s}`, d.Name, d.Host, source, connID), d.ID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func (r *bindingsRig) bindAlbums(t *testing.T, d model.Device, source string, ids ...uint) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"source": source, "album_ids": ids})
	rec := call(t, r.devices.UpdateDeviceAlbums, http.MethodPut, string(b), d.ID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func (r *bindingsRig) boundAlbums(t *testing.T, d model.Device, source string) []uint {
	t.Helper()
	rec := call(t, r.devices.GetDeviceAlbums, http.MethodGet, "", d.ID, "source="+source)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		AlbumIDs []uint `json:"album_ids"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.AlbumIDs
}

func (r *bindingsRig) device(t *testing.T, d model.Device) model.Device {
	t.Helper()
	var fresh model.Device
	require.NoError(t, r.db.First(&fresh, d.ID).Error)
	return fresh
}

// inUse decodes the 409 the dashboard shows as a dialog.
func inUse(t *testing.T, rec *httptest.ResponseRecorder) (string, []string) {
	t.Helper()
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var body struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Devices []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "in_use", body.Code)
	names := make([]string, 0, len(body.Devices))
	for _, d := range body.Devices {
		assert.NotZero(t, d.ID)
		names = append(names, d.Name)
	}
	return body.Error, names
}

// A frame's album selection survives a switch to another source, or to
// another connection of the same provider, and back; and while it is away
// only the current connection's selection is served from.
func TestFrameKeepsAlbumSelectionAcrossSwitches(t *testing.T) {
	r := newBindingsRig(t)
	home := r.connection(t, model.SourceImmich, "Home")
	parents := r.connection(t, model.SourceImmich, "Parents")
	nas := r.connection(t, model.SourceSynologyPhotos, "NAS")
	a := r.album(t, &home, model.SourceImmich, "a", "A")
	b := r.album(t, &home, model.SourceImmich, "b", "B")
	cc := r.album(t, &home, model.SourceImmich, "c", "C")
	p := r.album(t, &parents, model.SourceImmich, "p", "P")
	n := r.album(t, &nas, model.SourceSynologyPhotos, "n", "N")
	for _, al := range []model.Album{a, b, cc, p, n} {
		img := model.Image{Source: al.Source, ConnectionID: al.ConnectionID, ExternalID: "img-" + al.ExternalID, FilePath: "x"}
		require.NoError(t, r.db.Create(&img).Error)
		require.NoError(t, r.db.Create(&model.ImageAlbumMembership{ImageID: img.ID, AlbumID: al.ID}).Error)
	}
	frame := model.Device{Name: "Study", Host: "10.0.0.1"}
	require.NoError(t, r.db.Create(&frame).Error)

	// Immich / Home, albums A and C.
	r.updateDevice(t, frame, model.SourceImmich, &home)
	r.bindAlbums(t, frame, model.SourceImmich, a.ID, cc.ID)
	assert.ElementsMatch(t, []uint{a.ID, cc.ID}, r.boundAlbums(t, frame, model.SourceImmich))

	// Over to Synology with album N: the Immich selection is still there.
	r.updateDevice(t, frame, model.SourceSynologyPhotos, &nas)
	r.bindAlbums(t, frame, model.SourceSynologyPhotos, n.ID)
	assert.ElementsMatch(t, []uint{a.ID, cc.ID}, r.boundAlbums(t, frame, model.SourceImmich), "remembered while on Synology")
	d := r.device(t, frame)
	got, err := service.PickRandomDBPhotoForDevice(r.db, model.SourceSynologyPhotos, &d, "", nil)
	require.NoError(t, err)
	assert.Equal(t, &nas.ID, got.ConnectionID)

	// Back to Immich / Home: A and C again, and only they are served.
	r.updateDevice(t, frame, model.SourceImmich, &home)
	assert.ElementsMatch(t, []uint{a.ID, cc.ID}, r.boundAlbums(t, frame, model.SourceImmich))
	assert.Equal(t, []uint{n.ID}, r.boundAlbums(t, frame, model.SourceSynologyPhotos), "the Synology one is kept too")
	d = r.device(t, frame)
	for i := 0; i < 30; i++ {
		got, err := service.PickRandomDBPhotoForDevice(r.db, model.SourceImmich, &d, "", nil)
		require.NoError(t, err)
		var in []uint
		r.db.Model(&model.ImageAlbumMembership{}).Where("image_id = ?", got.ID).Pluck("album_id", &in)
		assert.NotContains(t, in, b.ID, "B was never selected")
		assert.NotContains(t, in, p.ID, "P is another connection's")
	}

	// Over to Immich / Parents: Home's A and C stay selected (unseen), and
	// nothing of Home is served meanwhile. Parents has no selection: all
	// of Parents.
	r.updateDevice(t, frame, model.SourceImmich, &parents)
	assert.ElementsMatch(t, []uint{a.ID, cc.ID}, r.boundAlbums(t, frame, model.SourceImmich), "remembered while on Parents")
	d = r.device(t, frame)
	for i := 0; i < 10; i++ {
		got, err := service.PickRandomDBPhotoForDevice(r.db, model.SourceImmich, &d, "", nil)
		require.NoError(t, err)
		assert.Equal(t, &parents.ID, got.ConnectionID)
	}
	// Selecting P on Parents keeps A and C for Home: the save sends them all.
	r.bindAlbums(t, frame, model.SourceImmich, a.ID, cc.ID, p.ID)
	r.updateDevice(t, frame, model.SourceImmich, &home)
	d = r.device(t, frame)
	for i := 0; i < 10; i++ {
		got, err := service.PickRandomDBPhotoForDevice(r.db, model.SourceImmich, &d, "", nil)
		require.NoError(t, err)
		assert.Equal(t, &home.ID, got.ConnectionID)
	}
}

// Deleting a connection a frame draws from is refused with the frames named,
// until the frame is moved; the frames' remembered selection of its albums
// then goes with it. A frame that merely remembers a selection on it does
// not hold it up.
func TestDeleteConnectionRefusedWhileFramesDrawFromIt(t *testing.T) {
	r := newBindingsRig(t)
	home := r.connection(t, model.SourceImmich, "Home")
	parents := r.connection(t, model.SourceImmich, "Parents")
	a := r.album(t, &home, model.SourceImmich, "a", "A")
	study := model.Device{Name: "Study", Host: "10.0.0.1"}
	hall := model.Device{Name: "Hall", Host: "10.0.0.2"}
	kitchen := model.Device{Name: "", Host: "10.0.0.3"}
	for _, d := range []*model.Device{&study, &hall, &kitchen} {
		require.NoError(t, r.db.Create(d).Error)
	}
	r.updateDevice(t, study, model.SourceImmich, &home) // bound to Home
	r.updateDevice(t, hall, model.SourceImmich, nil)    // any Immich, narrowed to A
	r.bindAlbums(t, hall, model.SourceImmich, a.ID)
	r.updateDevice(t, kitchen, model.SourceImmich, &parents) // on Parents, remembers A
	r.bindAlbums(t, kitchen, model.SourceImmich, a.ID)

	msg, names := inUse(t, call(t, r.connh.Delete, http.MethodDelete, "", home.ID, ""))
	assert.Equal(t, []string{"Study", "Hall"}, names)
	assert.Contains(t, msg, `the connection "Home" is used by 2 frames: Study, Hall`)
	_, err := r.conns.Store().Get(home.ID)
	require.NoError(t, err, "still there")

	// Study moves to Parents, Hall to Parents too: nothing draws from Home.
	r.updateDevice(t, study, model.SourceImmich, &parents)
	r.updateDevice(t, hall, model.SourceImmich, &parents)
	rec := call(t, r.connh.Delete, http.MethodDelete, "", home.ID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	// Without foreign keys in this in-memory schema the cascade is the
	// migration's business (covered in internal/db); the rows that remain
	// here are inert either way.
	for _, d := range []model.Device{study, hall, kitchen} {
		assert.Equal(t, &parents.ID, r.device(t, d).ImageConnectionID)
	}
}

// A gallery album a frame is limited to is refused the same way; a frame on
// another source only remembers it, and forgets it when it goes.
func TestDeleteGalleryAlbumRefusedWhileFramesDrawFromIt(t *testing.T) {
	r := newBindingsRig(t)
	trips := r.album(t, nil, model.SourceGallery, "gallery-1", "Trips")
	hall := model.Device{Name: "Hall", Host: "10.0.0.2"}
	study := model.Device{Name: "Study", Host: "10.0.0.1"}
	require.NoError(t, r.db.Create(&hall).Error)
	require.NoError(t, r.db.Create(&study).Error)
	r.updateDevice(t, hall, model.SourceGallery, nil)
	r.bindAlbums(t, hall, model.SourceGallery, trips.ID)
	r.updateDevice(t, study, model.SourceGallery, nil)
	r.bindAlbums(t, study, model.SourceGallery, trips.ID)
	r.updateDevice(t, study, model.SourceAIGeneration, nil) // away from the gallery

	msg, names := inUse(t, call(t, r.gallery.DeleteGalleryAlbum, http.MethodDelete, "", trips.ID, ""))
	assert.Equal(t, []string{"Hall"}, names, "Study only remembers the album")
	assert.Contains(t, msg, `the album "Trips" is used by 1 frame: Hall`)
	_, names = inUse(t, call(t, r.gallery.DeleteGalleryAlbum, http.MethodDelete, "", trips.ID, "photos=true"))
	assert.Equal(t, []string{"Hall"}, names)

	r.bindAlbums(t, hall, model.SourceGallery) // Hall: the whole gallery
	rec := call(t, r.gallery.DeleteGalleryAlbum, http.MethodDelete, "", trips.ID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, r.boundAlbums(t, study, model.SourceGallery), "the remembered selection went with the album")
}

// Removing a topic a frame is limited to is refused with the same answer,
// and the topic set is left as it was.
func TestRemoveTopicRefusedWhileFramesDrawFromIt(t *testing.T) {
	r := newBindingsRig(t)
	c := model.Connection{Source: model.SourceUnsplash, Name: "Unsplash", Enabled: true, State: "{}"}
	require.NoError(t, c.EncodeConfig(model.TopicConfig{APIKey: "k"}))
	require.NoError(t, r.db.Create(&c).Error)
	rec := call(t, r.connh.SetTopics, http.MethodPost, `{"topics":["sea","sky"]}`, c.ID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var sea model.Album
	require.NoError(t, r.db.Where("external_id = ?", "sea").First(&sea).Error)
	frame := model.Device{Name: "Study", Host: "10.0.0.1"}
	require.NoError(t, r.db.Create(&frame).Error)
	r.updateDevice(t, frame, model.SourceUnsplash, &c)
	r.bindAlbums(t, frame, model.SourceUnsplash, sea.ID)

	msg, names := inUse(t, call(t, r.connh.SetTopics, http.MethodPost, `{"topics":["sky"]}`, c.ID, ""))
	assert.Equal(t, []string{"Study"}, names)
	assert.Contains(t, msg, `the topic "sea" is used by 1 frame: Study`)
	rec = call(t, r.connh.Topics, http.MethodGet, "", c.ID, "")
	assert.Contains(t, rec.Body.String(), "sea")
	assert.Contains(t, rec.Body.String(), "sky")
}
