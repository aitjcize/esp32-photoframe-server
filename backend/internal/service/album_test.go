package service

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

var albumTestDBCounter atomic.Int64

func setupAlbumDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := albumTestDBCounter.Add(1)
	dsn := fmt.Sprintf("file:album_test_%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Image{}, &model.Album{},
		&model.ImageAlbumMembership{}, &model.DeviceAlbumMapping{}, &model.Device{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func mkImage(t *testing.T, db *gorm.DB, assetID string) model.Image {
	t.Helper()
	img := model.Image{ExternalID: assetID, Source: model.SourceImmich, Status: "pending"}
	if err := db.Create(&img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	return img
}

func mkAlbum(t *testing.T, db *gorm.DB, ext string) model.Album {
	t.Helper()
	a := model.Album{Source: model.SourceImmich, ExternalID: ext, Kind: model.AlbumKindReal, SyncEnabled: true}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create album: %v", err)
	}
	return a
}

func TestDeviceAlbumIDsAndAlbumScopedPick(t *testing.T) {
	db := setupAlbumDB(t)

	albumA := mkAlbum(t, db, "album-a")
	albumB := mkAlbum(t, db, "album-b")

	imgA := mkImage(t, db, "asset-a")
	imgB := mkImage(t, db, "asset-b")
	db.Create(&model.ImageAlbumMembership{ImageID: imgA.ID, AlbumID: albumA.ID})
	db.Create(&model.ImageAlbumMembership{ImageID: imgB.ID, AlbumID: albumB.ID})

	// device 1 -> album A, device 2 -> album B
	db.Create(&model.DeviceAlbumMapping{DeviceID: 1, AlbumID: albumA.ID})
	db.Create(&model.DeviceAlbumMapping{DeviceID: 2, AlbumID: albumB.ID})

	assert.ElementsMatch(t, []uint{albumA.ID}, DeviceAlbumIDs(db, 1, model.SourceImmich))
	assert.ElementsMatch(t, []uint{albumB.ID}, DeviceAlbumIDs(db, 2, model.SourceImmich))
	assert.Empty(t, DeviceAlbumIDs(db, 3, model.SourceImmich)) // unbound device

	// Each device sees only its album's photo.
	got, err := PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumA.ID}, nil)
	assert.NoError(t, err)
	assert.Equal(t, imgA.ID, got.ID)

	got, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumB.ID}, nil)
	assert.NoError(t, err)
	assert.Equal(t, imgB.ID, got.ID)

	// Excluding the only member of an album yields no photo (no cross-album leak).
	_, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", []uint{albumA.ID}, []uint{imgA.ID})
	assert.Error(t, err)

	// No album binding -> whole-source pool (legacy fallback).
	_, err = PickRandomDBPhotoForAlbums(db, model.SourceImmich, "", nil, nil)
	assert.NoError(t, err)
}

// A frame bound to one connection draws only from that connection's photos,
// narrowed further by its album bindings of that connection; album bindings
// left over from another connection do not empty its pool. An unbound frame
// still draws from every connection.
func TestPickRandomDBPhotoForDeviceHonoursTheConnectionBinding(t *testing.T) {
	db := setupAlbumDB(t)
	if err := db.AutoMigrate(&model.Connection{}); err != nil {
		t.Fatal(err)
	}
	home := model.Connection{Source: model.SourceImmich, Name: "Home", Enabled: true}
	nas := model.Connection{Source: model.SourceImmich, Name: "Parents", Enabled: true}
	db.Create(&home)
	db.Create(&nas)
	conn := func(c model.Connection) model.Image {
		img := model.Image{ExternalID: c.Name, Source: model.SourceImmich, Status: "pending", ConnectionID: &c.ID}
		db.Create(&img)
		return img
	}
	imgHome := conn(home)
	imgHome2 := model.Image{ExternalID: "home-2", Source: model.SourceImmich, Status: "pending", ConnectionID: &home.ID}
	db.Create(&imgHome2)
	imgNas := conn(nas)
	albumHome := model.Album{Source: model.SourceImmich, ExternalID: "trips", Kind: model.AlbumKindReal, SyncEnabled: true, ConnectionID: &home.ID}
	albumNas := model.Album{Source: model.SourceImmich, ExternalID: "family", Kind: model.AlbumKindReal, SyncEnabled: true, ConnectionID: &nas.ID}
	db.Create(&albumHome)
	db.Create(&albumNas)
	db.Create(&model.ImageAlbumMembership{ImageID: imgHome.ID, AlbumID: albumHome.ID})
	db.Create(&model.ImageAlbumMembership{ImageID: imgNas.ID, AlbumID: albumNas.ID})

	// Unbound: any connection.
	seen := map[uint]bool{}
	for i := 0; i < 40; i++ {
		got, err := PickRandomDBPhotoForDevice(db, model.SourceImmich, &model.Device{ID: 1}, "", nil)
		assert.NoError(t, err)
		seen[got.ID] = true
	}
	assert.True(t, seen[imgNas.ID] && (seen[imgHome.ID] || seen[imgHome2.ID]), "unbound frame draws from both connections: %v", seen)

	// Bound to the NAS: only its photo, even with no album bound.
	bound := &model.Device{ID: 2, ImageConnectionID: &nas.ID}
	got, err := PickRandomDBPhotoForDevice(db, model.SourceImmich, bound, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, imgNas.ID, got.ID)
	_, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, bound, "", []uint{imgNas.ID})
	assert.Error(t, err, "the other connection's photos are not a fallback")

	// Bound to Home with a Home album: only that album's photo, even with a
	// binding to a NAS album beside it.
	db.Create(&model.DeviceAlbumMapping{DeviceID: 3, AlbumID: albumHome.ID})
	db.Create(&model.DeviceAlbumMapping{DeviceID: 3, AlbumID: albumNas.ID})
	homeFrame := &model.Device{ID: 3, ImageConnectionID: &home.ID}
	for i := 0; i < 10; i++ {
		got, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, homeFrame, "", nil)
		assert.NoError(t, err)
		assert.Equal(t, imgHome.ID, got.ID)
	}
	// Only a remembered selection on the other connection: that plays no
	// part here, so the frame gets its whole bound connection.
	db.Create(&model.DeviceAlbumMapping{DeviceID: 4, AlbumID: albumNas.ID})
	got, err = PickRandomDBPhotoForDevice(db, model.SourceImmich, &model.Device{ID: 4, ImageConnectionID: &home.ID}, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, &home.ID, got.ConnectionID)

	// Frames that draw from a connection: bound to it, on "any" with one of
	// its albums selected, or showing its calendar. A remembered selection
	// on a frame that is bound elsewhere or on another source does not count.
	cal := model.Connection{Source: model.SourceGoogleCalendar, Name: "Cal", Enabled: true}
	db.Create(&cal)
	require.NoError(t, db.AutoMigrate(&model.Device{}))
	// The picker checks above used device ids without device rows; start
	// the frame list clean.
	require.NoError(t, db.Where("1 = 1").Delete(&model.DeviceAlbumMapping{}).Error)
	for _, d := range []model.Device{
		{Name: "bound", Host: "h1", Source: model.SourceImmich, ImageConnectionID: &nas.ID},
		{Name: "any-by-album", Host: "h2", Source: model.SourceImmich},
		{Name: "elsewhere", Host: "h3", Source: model.SourceImmich, ImageConnectionID: &home.ID},
		{Name: "other-source", Host: "h4", Source: model.SourceGallery},
		{Name: "calendar", Host: "h5", ShowCalendar: true, CalendarConnectionID: &cal.ID},
		{Name: "calendar-off", Host: "h6", CalendarConnectionID: &cal.ID},
	} {
		d := d
		require.NoError(t, db.Create(&d).Error)
		if d.Name != "bound" && d.Name != "calendar" && d.Name != "calendar-off" {
			db.Create(&model.DeviceAlbumMapping{DeviceID: d.ID, AlbumID: albumNas.ID})
		}
	}
	users, err := FramesUsingConnection(db, nas)
	require.NoError(t, err)
	assert.Equal(t, []string{"bound", "any-by-album"}, (&InUseError{Devices: users}).DeviceNames())
	users, err = FramesUsingConnection(db, cal)
	require.NoError(t, err)
	assert.Equal(t, []string{"calendar"}, (&InUseError{Devices: users}).DeviceNames(), "calendar switched off: only remembered")
	users, err = FramesUsingConnection(db, home)
	require.NoError(t, err)
	assert.Equal(t, []string{"elsewhere"}, (&InUseError{Devices: users}).DeviceNames(), "bound to it, no albums: the whole connection")
	users, err = FramesUsingAlbums(db, []uint{albumNas.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{"any-by-album"}, (&InUseError{Devices: users}).DeviceNames())
	assert.Equal(t, "10.0.0.4", (&InUseError{Devices: []model.Device{{Host: "10.0.0.4"}}}).DeviceNames()[0], "an unnamed frame goes by host")

	// The handler's check: a binding must name a connection of the frame's
	// source; built-in sources and 0 clear it.
	zero := uint(0)
	for _, tc := range []struct {
		source string
		id     *uint
		want   *uint
		bad    bool
	}{
		{model.SourceImmich, nil, nil, false},
		{model.SourceImmich, &zero, nil, false},
		{model.SourceImmich, &nas.ID, &nas.ID, false},
		{model.SourceGallery, &nas.ID, nil, false},
		{model.SourceSynologyPhotos, &nas.ID, nil, true},
		{model.SourceImmich, ptrUint(999), nil, true},
	} {
		got, err := ImageConnectionFor(db, tc.source, tc.id)
		if tc.bad {
			assert.Error(t, err, "%+v", tc)
			continue
		}
		assert.NoError(t, err, "%+v", tc)
		assert.Equal(t, tc.want, got, "%+v", tc)
	}
}

func ptrUint(v uint) *uint { return &v }

// Unticking an album in the connection card is refused while a frame on
// that connection (or on "any") is limited to it; a frame bound to another
// connection only remembers it and does not hold it. Other albums come and
// go as before.
func TestDeselectingAUsedAlbumIsRefused(t *testing.T) {
	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Connection{}, &model.Device{}))
	nas := model.Connection{Source: model.SourceSynologyPhotos, Name: "NAS", Enabled: true}
	other := model.Connection{Source: model.SourceSynologyPhotos, Name: "Other", Enabled: true}
	db.Create(&nas)
	db.Create(&other)
	sc := sourceScope{Source: model.SourceSynologyPhotos, ConnectionID: nas.ID}
	require.NoError(t, SetSyncAlbums(db, sc, []RemoteAlbum{{ExternalID: "1", Name: "Trips"}, {ExternalID: "2", Name: "Pets"}}))
	var trips model.Album
	require.NoError(t, db.Where("external_id = ?", "1").First(&trips).Error)
	hall := model.Device{Name: "Hall", Host: "h", Source: model.SourceSynologyPhotos, ImageConnectionID: &nas.ID}
	away := model.Device{Name: "Away", Host: "a", Source: model.SourceSynologyPhotos, ImageConnectionID: &other.ID}
	require.NoError(t, db.Create(&hall).Error)
	require.NoError(t, db.Create(&away).Error)
	db.Create(&model.DeviceAlbumMapping{DeviceID: hall.ID, AlbumID: trips.ID})
	db.Create(&model.DeviceAlbumMapping{DeviceID: away.ID, AlbumID: trips.ID})

	err := SetSyncAlbums(db, sc, []RemoteAlbum{{ExternalID: "2", Name: "Pets"}})
	var inUse *InUseError
	require.ErrorAs(t, err, &inUse)
	assert.Equal(t, []string{"Hall"}, inUse.DeviceNames(), "Away only remembers Trips")
	assert.Contains(t, err.Error(), `the album "Trips" is used by 1 frame: Hall`)
	var still model.Album
	require.NoError(t, db.First(&still, trips.ID).Error)
	assert.True(t, still.SyncEnabled, "the refusal changes nothing")

	require.NoError(t, SetSyncAlbums(db, sc, []RemoteAlbum{{ExternalID: "1", Name: "Trips"}}), "Pets is nobody's")
	require.NoError(t, db.Where("device_id = ?", hall.ID).Delete(&model.DeviceAlbumMapping{}).Error)
	require.NoError(t, SetSyncAlbums(db, sc, nil), "and once Hall is on the whole NAS, Trips can go too")
}
