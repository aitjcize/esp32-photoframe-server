package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
)

// authFrame is a frame with every endpoint behind the password gate, the
// way the firmware registers them: no credential or a wrong one gets a 401
// whatever the path.
type authFrame struct {
	mu        sync.Mutex
	password  string
	locked    bool // refusing password attempts: 429 to everything
	noAuthKey bool // firmware from before frame passwords
	// refuseAfter, when set, has the frame refuse (401) every request after
	// the first refuseAfter: its password changed under the server.
	refuseAfter int
	hits        []string
}

func (f *authFrame) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits = append(f.hits, r.Method+" "+r.URL.Path)
		if f.locked {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"too many wrong passwords, try again later"}`)
			return
		}
		_, pass, hasAuth := r.BasicAuth()
		changedUnderneath := f.refuseAfter > 0 && len(f.hits) > f.refuseAfter
		if changedUnderneath || (f.password != "" && (!hasAuth || pass != f.password)) {
			w.Header().Set("WWW-Authenticate", `Basic realm="ESP32 PhotoFrame"`)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"authentication required"}`)
			return
		}
		switch r.URL.Path {
		case "/api/system-info":
			fmt.Fprint(w, `{"device_name":"Frame","width":800,"height":480,"board_name":"test","version":"2.7.0"}`)
		case "/api/config":
			if r.Method != http.MethodGet {
				fmt.Fprint(w, `{"status":"success"}`)
				return
			}
			cfg := map[string]interface{}{"device_name": "Frame", "display_orientation": "landscape"}
			if !f.noAuthKey {
				cfg["http_auth_enabled"] = f.password != ""
			}
			json.NewEncoder(w).Encode(cfg)
		case "/api/settings/processing", "/api/settings/palette", "/api/display-image":
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *authFrame) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

// setupAuthFrameTest is setupFramePasswordTest for an authFrame.
func setupAuthFrameTest(t *testing.T, frame *authFrame, storedPassword string) (*DeviceService, *gorm.DB, uint) {
	t.Helper()
	n := framePwTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:frame_auth_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Device{}))

	srv := httptest.NewServer(frame.handler())
	t.Cleanup(srv.Close)

	d := model.Device{Name: "Frame", Host: strings.TrimPrefix(srv.URL, "http://"), HTTPPassword: storedPassword}
	require.NoError(t, db.Create(&d).Error)
	// Every test database numbers its devices from 1, and the answer clock
	// is per device id: a fresh device gets a fresh clock, as in production,
	// where ids are never reused.
	frameAuthClocks.Delete(d.ID)
	return NewDeviceService(DeviceServiceDeps{DB: db}), db, d.ID
}

func deviceRow(t *testing.T, db *gorm.DB, id uint) model.Device {
	t.Helper()
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	return d
}

// flagDevice puts the device in the state a refusal leaves it in.
func flagDevice(t *testing.T, db *gorm.DB, id uint, since time.Time) {
	t.Helper()
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).
		Updates(map[string]interface{}{"auth_required": true, "auth_failed_at": since}).Error)
}

func assertFlagged(t *testing.T, d model.Device) {
	t.Helper()
	assert.True(t, d.AuthRequired, "auth_required")
	if assert.NotNil(t, d.AuthFailedAt, "auth_failed_at") {
		assert.WithinDuration(t, time.Now(), *d.AuthFailedAt, time.Minute)
	}
}

func assertNotFlagged(t *testing.T, d model.Device) {
	t.Helper()
	assert.False(t, d.AuthRequired, "auth_required")
	assert.Nil(t, d.AuthFailedAt, "auth_failed_at")
}

func TestRefreshMarksDeviceWhenFrameRefusesPassword(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "stale")

	_, err := svc.RefreshDeviceFromHardware(id)
	require.Error(t, err)
	assert.Equal(t, 401, frameStatus(err))
	assertFlagged(t, deviceRow(t, db, id))
	// One request, not one per fetch: a refused password stays refused.
	assert.Equal(t, []string{"GET /api/system-info"}, frame.requests())
}

func TestRefreshClearsTheFlagOnceTheFrameAnswers(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "actual")
	flagDevice(t, db, id, time.Now().Add(-time.Hour))

	d, err := svc.RefreshDeviceFromHardware(id)
	require.NoError(t, err)
	assertNotFlagged(t, deviceRow(t, db, id))
	// The copy handed back agrees with the row, not with the state before
	// the fetches.
	assertNotFlagged(t, *d)
}

// The frame's password changes partway through a refresh: the required
// fetches went through, an optional one was refused. The refresh succeeds,
// and the copy it returns reports the refusal the row now records.
func TestRefreshReportsARefusalOfAnOptionalFetch(t *testing.T) {
	frame := &authFrame{password: "actual", refuseAfter: 2}
	svc, db, id := setupAuthFrameTest(t, frame, "actual")

	d, err := svc.RefreshDeviceFromHardware(id)
	require.NoError(t, err)
	assert.Equal(t, "Frame", d.Name, "the required fetches went through")
	assert.Equal(t, []string{
		"GET /api/system-info", "GET /api/config",
		"GET /api/settings/processing", "GET /api/settings/palette",
	}, frame.requests())
	assertFlagged(t, deviceRow(t, db, id))
	assertFlagged(t, *d)
}

// Two requests out at once: whichever observer runs first, the answer to
// the later-sent request wins. A 200 to a request sent before the latest
// refusal came in does not clear it; a 401 that came in before an accepted
// request was sent does not stand against it.
func TestConcurrentAnswersSettleByWhenTheyWereSent(t *testing.T) {
	// Fixed, slightly past times, so the tests' own clocks do not interfere.
	base := time.Now().Add(-time.Second)
	answer := func(status int, sent, received time.Duration) photoframe.Answer {
		return photoframe.Answer{Status: status, SentAt: base.Add(sent), ReceivedAt: base.Add(received)}
	}

	t.Run("refusal recorded first", func(t *testing.T) {
		_, db, id := setupAuthFrameTest(t, &authFrame{}, "pw")
		observe := frameAuthObserver(db, id)
		observe(answer(http.StatusUnauthorized, 0, 10*time.Millisecond))
		assertFlagged(t, deviceRow(t, db, id))
		// Sent before the refusal came in: possibly gated before it.
		observe(answer(http.StatusOK, 5*time.Millisecond, 12*time.Millisecond))
		assertFlagged(t, deviceRow(t, db, id))
		// Sent after it: the frame accepts the password now.
		observe(answer(http.StatusOK, 20*time.Millisecond, 25*time.Millisecond))
		assertNotFlagged(t, deviceRow(t, db, id))
	})

	t.Run("acceptance recorded first", func(t *testing.T) {
		_, db, id := setupAuthFrameTest(t, &authFrame{}, "pw")
		observe := frameAuthObserver(db, id)
		observe(answer(http.StatusOK, 20*time.Millisecond, 25*time.Millisecond))
		// Came in before that accepted request went out: superseded.
		observe(answer(http.StatusUnauthorized, 0, 10*time.Millisecond))
		assertNotFlagged(t, deviceRow(t, db, id))
		// Came in after it: the frame refuses the password now.
		observe(answer(http.StatusUnauthorized, 28*time.Millisecond, 30*time.Millisecond))
		assertFlagged(t, deviceRow(t, db, id))
	})
}

// The check SetHTTPPassword makes holds the device's exclusive lock, which
// every request to the frame waits on: a frame that takes the connection
// and never answers must not hold them for the shared client's two minutes.
func TestSetHTTPPasswordCheckIsBounded(t *testing.T) {
	stall := make(chan struct{})
	mute := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-stall
	}))
	t.Cleanup(func() {
		close(stall)
		mute.Close()
	})
	svc, db, id := setupAuthFrameTest(t, &authFrame{}, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).
		Update("host", strings.TrimPrefix(mute.URL, "http://")).Error)
	was := frameCheckTimeout
	frameCheckTimeout = 200 * time.Millisecond
	t.Cleanup(func() { frameCheckTimeout = was })

	start := time.Now()
	unverified, err := svc.SetHTTPPassword(id, "typed")
	require.NoError(t, err)
	require.NotNil(t, unverified)
	assert.Equal(t, FrameUnreachable, unverified.Kind)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, "typed", deviceRow(t, db, id).HTTPPassword, "stored for the frame's next wake")
}

// The flag dates the first refusal, so the webapp can tell a new run of
// refusals from the one it already asked about.
func TestAuthFailedAtKeepsTheFirstRefusal(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "stale")
	first := time.Now().Add(-time.Hour).Truncate(time.Second)
	flagDevice(t, db, id, first)

	_, err := svc.RefreshDeviceFromHardware(id)
	require.Error(t, err)
	d := deviceRow(t, db, id)
	assert.True(t, d.AuthRequired)
	require.NotNil(t, d.AuthFailedAt)
	assert.True(t, d.AuthFailedAt.Equal(first), "auth_failed_at moved to %v", d.AuthFailedAt)
}

// A lockout says nothing about the password: the frame did not check it.
func TestLockoutLeavesTheFlagAlone(t *testing.T) {
	for _, flagged := range []bool{false, true} {
		t.Run(fmt.Sprintf("flagged=%v", flagged), func(t *testing.T) {
			frame := &authFrame{password: "actual", locked: true}
			svc, db, id := setupAuthFrameTest(t, frame, "actual")
			if flagged {
				flagDevice(t, db, id, time.Now())
			}
			_, err := svc.RefreshDeviceFromHardware(id)
			require.Error(t, err)
			assert.Equal(t, 429, frameStatus(err))
			assert.Equal(t, flagged, deviceRow(t, db, id).AuthRequired)
		})
	}
}

func TestImagePushStopsAtRefusedPasswordAndMarksDevice(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "stale")
	d := deviceRow(t, db, id)

	err := svc.PushToHost(&d, filepath.Join(t.TempDir(), "missing.jpg"), nil)
	require.Error(t, err)
	assert.Equal(t, 401, frameStatus(err))
	assertFlagged(t, deviceRow(t, db, id))
	// Nothing was rendered or pushed: the system-info fetch settled it.
	assert.Equal(t, []string{"GET /api/system-info"}, frame.requests())
}

func TestAddDeviceMarksAFrameThatRefusesTheProbe(t *testing.T) {
	frame := &authFrame{password: "actual"}
	srv := httptest.NewServer(frame.handler())
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	db, err := gorm.Open(sqlite.Open("file:frame_auth_add?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Device{}))
	svc := NewDeviceService(DeviceServiceDeps{DB: db})

	for name, password := range map[string]string{"none": "", "wrong": "nope"} {
		t.Run(name, func(t *testing.T) {
			d, err := svc.AddDevice(host, password, false, false, false, false, 0, 0, "", "", false, "", "")
			require.NoError(t, err)
			assertFlagged(t, deviceRow(t, db, d.ID))
			assert.Equal(t, host, d.Name, "placeholder name, as for an unreachable frame")
		})
	}
	t.Run("right", func(t *testing.T) {
		d, err := svc.AddDevice(host, "actual", false, false, false, false, 0, 0, "", "", false, "", "")
		require.NoError(t, err)
		assertNotFlagged(t, deviceRow(t, db, d.ID))
		assert.Equal(t, "Frame", d.Name)
	})
}

func TestSetHTTPPasswordChecksWithTheFrame(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		frame := &authFrame{password: "actual"}
		svc, db, id := setupAuthFrameTest(t, frame, "stale")
		flagDevice(t, db, id, time.Now())

		unverified, err := svc.SetHTTPPassword(id, "actual")
		require.NoError(t, err)
		assert.Nil(t, unverified)
		d := deviceRow(t, db, id)
		assert.Equal(t, "actual", d.HTTPPassword)
		assertNotFlagged(t, d)
		// Checked with the new password, never the stored one.
		assert.Equal(t, []string{"GET /api/config"}, frame.requests())
	})

	t.Run("rejected", func(t *testing.T) {
		frame := &authFrame{password: "actual"}
		svc, db, id := setupAuthFrameTest(t, frame, "stale")
		since := time.Now().Add(-time.Hour).Truncate(time.Second)
		flagDevice(t, db, id, since)

		_, err := svc.SetHTTPPassword(id, "wrong")
		var fpe *FramePasswordError
		require.ErrorAs(t, err, &fpe)
		assert.Equal(t, FrameWrongPassword, fpe.Kind)
		d := deviceRow(t, db, id)
		assert.Equal(t, "stale", d.HTTPPassword, "a rejected password is not stored")
		// The check said nothing about the stored password: the flag stays
		// as it was, first refusal and all.
		assert.True(t, d.AuthRequired)
		require.NotNil(t, d.AuthFailedAt)
		assert.True(t, d.AuthFailedAt.Equal(since))
	})

	t.Run("unreachable", func(t *testing.T) {
		svc, db, id := setupAuthFrameTest(t, &authFrame{}, "stale")
		flagDevice(t, db, id, time.Now())
		require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("host", deadHost(t)).Error)

		unverified, err := svc.SetHTTPPassword(id, "typed")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameUnreachable, unverified.Kind)
		d := deviceRow(t, db, id)
		assert.Equal(t, "typed", d.HTTPPassword, "stored for the frame's next wake")
		assertNotFlagged(t, d)
	})

	t.Run("locked out", func(t *testing.T) {
		frame := &authFrame{password: "actual", locked: true}
		svc, db, id := setupAuthFrameTest(t, frame, "stale")

		unverified, err := svc.SetHTTPPassword(id, "actual")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameLockedOut, unverified.Kind)
		assert.Equal(t, "30", unverified.RetryAfter)
		assert.Equal(t, "actual", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("open frame", func(t *testing.T) {
		svc, db, id := setupAuthFrameTest(t, &authFrame{}, "")

		unverified, err := svc.SetHTTPPassword(id, "ahead-of-time")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameOpen, unverified.Kind)
		assert.Equal(t, "ahead-of-time", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("old firmware", func(t *testing.T) {
		svc, db, id := setupAuthFrameTest(t, &authFrame{noAuthKey: true}, "")

		unverified, err := svc.SetHTTPPassword(id, "typed")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameUnsupported, unverified.Kind)
		assert.Equal(t, "typed", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("no host", func(t *testing.T) {
		svc, db, id := setupAuthFrameTest(t, &authFrame{}, "")
		require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("host", "").Error)

		unverified, err := svc.SetHTTPPassword(id, "typed")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameNoHost, unverified.Kind)
		assert.Equal(t, "typed", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("forget while the frame still asks", func(t *testing.T) {
		frame := &authFrame{password: "actual"}
		svc, db, id := setupAuthFrameTest(t, frame, "actual")

		unverified, err := svc.SetHTTPPassword(id, "")
		require.NoError(t, err)
		require.NotNil(t, unverified)
		assert.Equal(t, FrameWrongPassword, unverified.Kind)
		assert.True(t, unverified.NoneStored)
		d := deviceRow(t, db, id)
		assert.Equal(t, "", d.HTTPPassword, "forgotten as asked")
		assertFlagged(t, d)
	})

	t.Run("forget an open frame", func(t *testing.T) {
		svc, db, id := setupAuthFrameTest(t, &authFrame{}, "was-set")
		flagDevice(t, db, id, time.Now())

		unverified, err := svc.SetHTTPPassword(id, "")
		require.NoError(t, err)
		assert.Nil(t, unverified)
		d := deviceRow(t, db, id)
		assert.Equal(t, "", d.HTTPPassword)
		assertNotFlagged(t, d)
	})

	t.Run("unknown device", func(t *testing.T) {
		svc, _, _ := setupAuthFrameTest(t, &authFrame{}, "")
		_, err := svc.SetHTTPPassword(9999, "typed")
		assert.ErrorIs(t, err, ErrDeviceNotFound)
	})
}

// A request in flight with the old password finishes, and records what the
// frame made of it, before the new password is stored: its 401 cannot land
// on the new password.
func TestSetHTTPPasswordWaitsForRequestsInFlight(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "stale")

	client, release, err := FrameClient(db, id)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		_, _ = svc.SetHTTPPassword(id, "actual")
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("the password was stored while a request held the old one")
	case <-time.After(100 * time.Millisecond):
	}
	// The held client's refusal lands first...
	_, err = client.FetchSystemInfo()
	assert.Equal(t, 401, frameStatus(err))
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("storing the password did not proceed after release")
	}
	// ...and the store, with the frame's confirmation, has the last word.
	d := deviceRow(t, db, id)
	assert.Equal(t, "actual", d.HTTPPassword)
	assertNotFlagged(t, d)
}

// A save of the device's settings leaves the flag to the frame's answers --
// unless it moves the device to another host, whose frame has refused
// nothing yet.
func TestUpdateDeviceKeepsTheFlagUnlessTheHostChanges(t *testing.T) {
	svc, db, id := setupAuthFrameTest(t, &authFrame{}, "pw")
	flagDevice(t, db, id, time.Now())
	host := deviceRow(t, db, id).Host

	d, err := svc.UpdateDevice(id, "Renamed", host, "", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
	require.NoError(t, err)
	assert.True(t, d.AuthRequired, "the returned device")
	assert.True(t, deviceRow(t, db, id).AuthRequired)

	d, err = svc.UpdateDevice(id, "Renamed", deadHost(t), "", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
	require.NoError(t, err)
	assertNotFlagged(t, *d)
	assertNotFlagged(t, deviceRow(t, db, id))
}

// An edit that keeps the host does not wait behind a request in flight: a
// new name must not stall for as long as a push to a frame that has stopped
// answering.
func TestUpdateDeviceWithoutHostChangeDoesNotWaitForRequestsInFlight(t *testing.T) {
	svc, db, id := setupAuthFrameTest(t, &authFrame{}, "pw")
	host := deviceRow(t, db, id).Host

	_, release, err := FrameClient(db, id)
	require.NoError(t, err)
	defer release()

	done := make(chan error, 1)
	go func() {
		_, err := svc.UpdateDevice(id, "Renamed", host, "", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("an edit that keeps the host waited on a request in flight")
	}
	assert.Equal(t, "Renamed", deviceRow(t, db, id).Name)
}

// A request already on its way to the old host finishes, and records the old
// frame's refusal, before a move to a new host resets the flag: the refusal
// cannot land on the new host.
func TestUpdateDeviceHostChangeWaitsForRequestsInFlight(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "stale")

	client, release, err := FrameClient(db, id)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		_, _ = svc.UpdateDevice(id, "Frame", deadHost(t), "", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("the host was changed while a request held the old one")
	case <-time.After(100 * time.Millisecond):
	}
	_, err = client.FetchSystemInfo()
	assert.Equal(t, 401, frameStatus(err))
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the host change did not proceed after release")
	}
	assertNotFlagged(t, deviceRow(t, db, id))
}

func TestChangeFramePasswordKeepsTheFlagInStep(t *testing.T) {
	t.Run("stored password refused", func(t *testing.T) {
		frame := &fakeFrame{password: "actual"}
		svc, db, id := setupFramePasswordTest(t, frame, "stale")

		_, err := svc.ChangeFramePassword(id, "new", "")
		require.Error(t, err)
		assertFlagged(t, deviceRow(t, db, id))
	})
	t.Run("change taken", func(t *testing.T) {
		frame := &fakeFrame{password: "old"}
		svc, db, id := setupFramePasswordTest(t, frame, "old")
		flagDevice(t, db, id, time.Now())

		_, err := svc.ChangeFramePassword(id, "new", "")
		require.NoError(t, err)
		d := deviceRow(t, db, id)
		assert.Equal(t, "new", d.HTTPPassword)
		assertNotFlagged(t, d)
	})
}

// Every request the client makes is a chance to notice: the observer sits
// under all of them.
func TestFrameClientRecordsEveryAnswer(t *testing.T) {
	frame := &authFrame{password: "actual"}
	svc, db, id := setupAuthFrameTest(t, frame, "actual")
	_ = svc

	client, release, err := FrameClient(db, id)
	require.NoError(t, err)
	defer release()

	frame.mu.Lock()
	frame.password = "changed-on-the-frame"
	frame.mu.Unlock()
	_, err = client.FetchPalette()
	assert.Equal(t, 401, frameStatus(err))
	assertFlagged(t, deviceRow(t, db, id))

	frame.mu.Lock()
	frame.password = "actual"
	frame.mu.Unlock()
	require.NoError(t, client.PushProcessingSettings([]byte(`{}`)))
	assertNotFlagged(t, deviceRow(t, db, id))
}
