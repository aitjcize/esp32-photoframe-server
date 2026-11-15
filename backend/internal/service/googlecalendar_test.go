package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

var calendarTestDBCounter atomic.Int64

func setupCalendarDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := calendarTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:calendar_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Connection{}, &model.Image{}, &model.Album{},
		&model.ImageAlbumMembership{}, &model.Device{}))
	return db
}

// signIn gives a connection a token as the OAuth exchange would.
func signIn(t *testing.T, db *gorm.DB, id uint, tag string) {
	t.Helper()
	st := &connectionTokenStore{conns: NewConnectionStore(db), id: id}
	require.NoError(t, st.SaveToken(&oauth2.Token{
		AccessToken: "acc-" + tag, RefreshToken: "ref-" + tag, Expiry: time.Now().Add(time.Hour),
	}))
}

// A calendar connection is a Google account like a Photos one: the manager
// builds a calendar service for it, it starts a login with the calendar
// scopes, and it has no photos to sync.
func TestCalendarConnectionIsAGoogleAccount(t *testing.T) {
	db := setupCalendarDB(t)
	photosCfg := staticGoogleConfig{id: "cid", secret: "sec"}
	m := NewConnectionManager(db, photosCfg, photosCfg, nil)
	m.Start()
	cal := mkConnection(t, db, model.SourceGoogleCalendar, "Family", struct{}{})

	svc, err := m.Calendar(cal.ID)
	require.NoError(t, err)
	assert.Equal(t, cal.ID, svc.ConnectionID())
	assert.False(t, svc.Connected())
	_, err = svc.Calendars()
	assert.ErrorIs(t, err, ErrCalendarNotConnected)

	url, err := m.BeginGoogleAuth(cal.ID, "http://frame.local/api/auth/google/callback")
	require.NoError(t, err)
	assert.Contains(t, url, "client_id=cid")

	signIn(t, db, cal.ID, "cal")
	assert.True(t, svc.Connected())

	// Not a photo source: no sync backend, and the Photos accessor refuses it.
	_, err = m.Backend(cal.ID)
	assert.Error(t, err)
	_, err = m.Google(cal.ID)
	assert.Error(t, err)

	// Logout forgets the token through the shared account path.
	require.NoError(t, m.GoogleLogout(cal.ID))
	assert.False(t, svc.Connected())
}

// A frame shows the calendar of the account it is bound to; without a
// binding it gets the first connected, enabled account, as every frame did
// when there was only one; with none connected there is no calendar.
func TestCalendarForDevice(t *testing.T) {
	db := setupCalendarDB(t)
	cfg := staticGoogleConfig{id: "cid", secret: "sec"}
	m := NewConnectionManager(db, cfg, cfg, nil)
	m.Start()

	_, err := m.CalendarForDevice(model.Device{ShowCalendar: true})
	assert.ErrorIs(t, err, ErrCalendarNotConnected, "no accounts at all")

	a := mkConnection(t, db, model.SourceGoogleCalendar, "A", struct{}{})
	b := mkConnection(t, db, model.SourceGoogleCalendar, "B", struct{}{})
	_, err = m.CalendarForDevice(model.Device{ShowCalendar: true})
	assert.ErrorIs(t, err, ErrCalendarNotConnected, "accounts without tokens do not count")

	signIn(t, db, b.ID, "b")
	svc, err := m.CalendarForDevice(model.Device{ShowCalendar: true})
	require.NoError(t, err)
	assert.Equal(t, b.ID, svc.ConnectionID(), "falls back to the first connected account")

	signIn(t, db, a.ID, "a")
	svc, err = m.CalendarForDevice(model.Device{ShowCalendar: true})
	require.NoError(t, err)
	assert.Equal(t, a.ID, svc.ConnectionID(), "the lowest id wins once connected")

	// A disabled account is skipped by the fallback but still served when
	// a frame is bound to it explicitly.
	require.NoError(t, db.Model(&model.Connection{}).Where("id = ?", a.ID).Update("enabled", false).Error)
	svc, err = m.CalendarForDevice(model.Device{ShowCalendar: true})
	require.NoError(t, err)
	assert.Equal(t, b.ID, svc.ConnectionID())
	svc, err = m.CalendarForDevice(model.Device{ShowCalendar: true, CalendarConnectionID: &a.ID})
	require.NoError(t, err)
	assert.Equal(t, a.ID, svc.ConnectionID())

	// Bound to a Photos account: refused rather than silently redirected.
	ph := mkConnection(t, db, model.SourceGooglePhotos, "Photos", struct{}{})
	_, err = m.CalendarForDevice(model.Device{ShowCalendar: true, CalendarConnectionID: &ph.ID})
	assert.Error(t, err)
}

// Deleting a calendar account unbinds the frames that showed it, so they fall
// back to another account instead of pointing at a missing row. Deleting a
// photo connection leaves calendar bindings alone.
func TestDeleteCalendarConnectionUnbindsDevices(t *testing.T) {
	db := setupCalendarDB(t)
	cfg := staticGoogleConfig{id: "cid", secret: "sec"}
	m := NewConnectionManager(db, cfg, cfg, NewPickerService(db, t.TempDir()))
	m.Start()
	a := mkConnection(t, db, model.SourceGoogleCalendar, "A", struct{}{})
	b := mkConnection(t, db, model.SourceGoogleCalendar, "B", struct{}{})
	ph := mkConnection(t, db, model.SourceGooglePhotos, "Photos", struct{}{})

	d1 := model.Device{Name: "one", Host: "1.1.1.1", ShowCalendar: true, CalendarConnectionID: &a.ID, CalendarID: "private@a"}
	d2 := model.Device{Name: "two", Host: "1.1.1.2", ShowCalendar: true, CalendarConnectionID: &b.ID, CalendarID: "team@b"}
	require.NoError(t, db.Create(&d1).Error)
	require.NoError(t, db.Create(&d2).Error)

	require.NoError(t, m.Delete(ph.ID))
	var got model.Device
	require.NoError(t, db.First(&got, d1.ID).Error)
	require.NotNil(t, got.CalendarConnectionID)
	assert.Equal(t, a.ID, *got.CalendarConnectionID)

	require.NoError(t, m.Delete(a.ID))
	// Fresh structs: gorm folds a loaded primary key into the next lookup.
	var one, two model.Device
	require.NoError(t, db.First(&one, d1.ID).Error)
	assert.Nil(t, one.CalendarConnectionID, "the frame on the deleted account is unbound")
	assert.Equal(t, "", one.CalendarID, "and forgets the calendar only that account could read")
	require.NoError(t, db.First(&two, d2.ID).Error)
	require.NotNil(t, two.CalendarConnectionID)
	assert.Equal(t, b.ID, *two.CalendarConnectionID, "other frames keep their account")
	assert.Equal(t, "team@b", two.CalendarID)

	_, err := m.Calendar(a.ID)
	assert.True(t, errors.Is(err, ErrConnectionNotFound), "got %v", err)
}
