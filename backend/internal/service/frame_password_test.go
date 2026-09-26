package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// fakeFrame mimics the firmware's password gate and its /api/config.
type fakeFrame struct {
	mu        sync.Mutex
	password  string
	noAuthKey bool // firmware from before frame passwords
	// patchStatus, when set, answers the PATCH with it instead of applying.
	patchStatus int
	patchBody   string
	retryAfter  string
	// dropPatchResponse applies the PATCH, then closes the connection
	// without answering: a lost response.
	dropPatchResponse bool
	// acceptButIgnore answers the PATCH 200 without applying it.
	acceptButIgnore bool
	// goneAfterPatch drops every request once a PATCH has come in: the
	// frame went away mid-change.
	goneAfterPatch bool

	patches      []string // Authorization password of each PATCH
	patchBodies  []string
	configGets   atomic.Int32
	lastGetCreds []string
}

func (f *fakeFrame) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.goneAfterPatch && len(f.patches) > 0 {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			conn.Close()
			return
		}
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		_, pass, hasAuth := r.BasicAuth()
		if f.password != "" && (!hasAuth || pass != f.password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="ESP32 PhotoFrame"`)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"authentication required"}`)
			return
		}
		switch r.Method {
		case http.MethodGet:
			f.configGets.Add(1)
			cred := "<none>"
			if hasAuth {
				cred = pass
			}
			f.lastGetCreds = append(f.lastGetCreds, cred)
			cfg := map[string]interface{}{"device_name": "Frame"}
			if !f.noAuthKey {
				cfg["http_auth_enabled"] = f.password != ""
			}
			json.NewEncoder(w).Encode(cfg)
		case http.MethodPatch:
			cred := "<none>"
			if hasAuth {
				cred = pass
			}
			f.patches = append(f.patches, cred)
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			b, _ := json.Marshal(body)
			f.patchBodies = append(f.patchBodies, string(b))
			if f.patchStatus != 0 {
				if f.retryAfter != "" {
					w.Header().Set("Retry-After", f.retryAfter)
				}
				w.WriteHeader(f.patchStatus)
				fmt.Fprint(w, f.patchBody)
				return
			}
			if pw, ok := body["http_password"].(string); ok && !f.acceptButIgnore {
				f.password = pw
			}
			if f.dropPatchResponse {
				conn, _, err := w.(http.Hijacker).Hijack()
				require.NoError(t, err)
				conn.Close()
				return
			}
			fmt.Fprint(w, `{"status":"success"}`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func (f *fakeFrame) current() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.password
}

func (f *fakeFrame) getCreds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lastGetCreds...)
}

func (f *fakeFrame) sentBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.patchBodies...)
}

func (f *fakeFrame) patchCreds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.patches...)
}

var framePwTestDBCounter atomic.Int64

// setupFramePasswordTest returns a device service whose one device points at
// a fake frame with framePassword set, and stores storedPassword for it.
func setupFramePasswordTest(t *testing.T, frame *fakeFrame, storedPassword string) (*DeviceService, *gorm.DB, uint) {
	t.Helper()
	n := framePwTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:frame_pw_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Device{}))

	srv := httptest.NewServer(frame.handler(t))
	t.Cleanup(srv.Close)

	d := model.Device{Name: "Frame", Host: strings.TrimPrefix(srv.URL, "http://"), HTTPPassword: storedPassword}
	require.NoError(t, db.Create(&d).Error)
	// A fresh device gets a fresh answer clock (see setupAuthFrameTest).
	frameAuthClocks.Delete(d.ID)
	return NewDeviceService(DeviceServiceDeps{DB: db}), db, d.ID
}

func storedPassword(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	return d.HTTPPassword
}

func TestChangeFramePasswordStoresNewAfterFrameAccepts(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	verified, err := svc.ChangeFramePassword(id, "new", "")
	require.NoError(t, err)
	assert.True(t, verified)
	assert.Equal(t, "new", frame.current())
	assert.Equal(t, "new", storedPassword(t, db, id))
	// The PATCH authenticates with the OLD password; the new one only
	// applies to later requests.
	assert.Equal(t, []string{"old"}, frame.patchCreds())
	assert.Equal(t, []string{`{"http_password":"new"}`}, frame.sentBodies())
	// Pre-check with the old password, read-back with the new one.
	assert.Equal(t, []string{"old", "new"}, frame.getCreds())
}

func TestChangeFramePasswordTurnsOnForOpenFrame(t *testing.T) {
	frame := &fakeFrame{}
	svc, db, id := setupFramePasswordTest(t, frame, "")

	verified, err := svc.ChangeFramePassword(id, "s3cret", "")
	require.NoError(t, err)
	assert.True(t, verified)
	assert.Equal(t, "s3cret", frame.current())
	assert.Equal(t, "s3cret", storedPassword(t, db, id))
	assert.Equal(t, []string{"<none>"}, frame.patchCreds())
}

func TestChangeFramePasswordTurnsOff(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	verified, err := svc.ChangeFramePassword(id, "", "")
	require.NoError(t, err)
	assert.True(t, verified)
	assert.Equal(t, "", frame.current())
	assert.Equal(t, "", storedPassword(t, db, id))
	assert.Equal(t, []string{"old"}, frame.patchCreds())
	// The read-back of an open frame sends no credential at all.
	assert.Equal(t, []string{"old", "<none>"}, frame.getCreds())
}

func TestChangeFramePasswordWrongStoredPasswordLeavesItUnchanged(t *testing.T) {
	frame := &fakeFrame{password: "actual"}
	svc, db, id := setupFramePasswordTest(t, frame, "stale")

	_, err := svc.ChangeFramePassword(id, "new", "")
	var fpe *FramePasswordError
	require.ErrorAs(t, err, &fpe)
	assert.Equal(t, FrameWrongPassword, fpe.Kind)
	assert.False(t, fpe.NoneStored)
	assert.Equal(t, "stale", storedPassword(t, db, id))
	assert.Equal(t, "actual", frame.current())
	// The pre-check caught it: no PATCH was even tried.
	assert.Empty(t, frame.patchCreds())
}

func TestChangeFramePasswordNoneStoredForProtectedFrame(t *testing.T) {
	frame := &fakeFrame{password: "actual"}
	svc, db, id := setupFramePasswordTest(t, frame, "")

	_, err := svc.ChangeFramePassword(id, "new", "")
	var fpe *FramePasswordError
	require.ErrorAs(t, err, &fpe)
	assert.Equal(t, FrameWrongPassword, fpe.Kind)
	assert.True(t, fpe.NoneStored)
	assert.Equal(t, "", storedPassword(t, db, id))
}

func TestChangeFramePasswordPatchFailuresLeaveItUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		wantKind   FramePasswordFailure
		wantInMsg  string
	}{
		{"401", 401, `{"error":"authentication required"}`, "", FrameWrongPassword, "rejected the password"},
		{"429", 429, `{"error":"too many wrong passwords, try again later"}`, "30", FrameLockedOut, "too many wrong"},
		{"400 too long", 400, `{"status":"error","message":"Device password is too long (max 63 bytes)"}`, "", FrameRefused, "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := &fakeFrame{password: "old", patchStatus: tt.status, patchBody: tt.body, retryAfter: tt.retryAfter}
			svc, db, id := setupFramePasswordTest(t, frame, "old")

			_, err := svc.ChangeFramePassword(id, "new", "")
			var fpe *FramePasswordError
			require.ErrorAs(t, err, &fpe)
			assert.Equal(t, tt.wantKind, fpe.Kind)
			assert.Equal(t, tt.retryAfter, fpe.RetryAfter)
			assert.Contains(t, err.Error(), tt.wantInMsg)
			assert.Equal(t, "old", storedPassword(t, db, id))
			assert.Equal(t, []string{"old"}, frame.patchCreds())
		})
	}
}

// deadHost returns a host:port on the loopback that nothing listens on, so
// a connection to it is refused at once.
func deadHost(t *testing.T) string {
	t.Helper()
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	return strings.TrimPrefix(dead.URL, "http://")
}

func TestChangeFramePasswordUnreachableLeavesItUnchanged(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")
	// Point the device at a port nothing listens on.
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("host", deadHost(t)).Error)

	_, err := svc.ChangeFramePassword(id, "new", "")
	var fpe *FramePasswordError
	require.ErrorAs(t, err, &fpe)
	assert.Equal(t, FrameUnreachable, fpe.Kind)
	assert.Equal(t, "old", storedPassword(t, db, id))
}

func TestChangeFramePasswordLostResponseChecksWhatTheFrameHas(t *testing.T) {
	t.Run("applied", func(t *testing.T) {
		frame := &fakeFrame{password: "old", dropPatchResponse: true}
		svc, db, id := setupFramePasswordTest(t, frame, "old")

		verified, err := svc.ChangeFramePassword(id, "new", "")
		require.NoError(t, err)
		assert.True(t, verified)
		assert.Equal(t, "new", storedPassword(t, db, id))
	})
	t.Run("not applied", func(t *testing.T) {
		frame := &fakeFrame{password: "old", dropPatchResponse: true, acceptButIgnore: true}
		svc, db, id := setupFramePasswordTest(t, frame, "old")

		_, err := svc.ChangeFramePassword(id, "new", "")
		var fpe *FramePasswordError
		require.ErrorAs(t, err, &fpe)
		assert.Equal(t, FrameUnreachable, fpe.Kind)
		assert.Equal(t, "old", storedPassword(t, db, id))
	})
}

// No answer to the change and none to the check after it: the outcome is
// unknown, which must not be reported as "nothing changed".
func TestChangeFramePasswordNoAnswerAtAllIsUnknown(t *testing.T) {
	frame := &fakeFrame{password: "old", dropPatchResponse: true, goneAfterPatch: true}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	_, err := svc.ChangeFramePassword(id, "new", "")
	var fpe *FramePasswordError
	require.ErrorAs(t, err, &fpe)
	assert.Equal(t, FrameOutcomeUnknown, fpe.Kind)
	assert.Equal(t, "old", storedPassword(t, db, id))
}

func TestChangeFramePasswordReadBackRejectionKeepsOldPassword(t *testing.T) {
	t.Run("401", func(t *testing.T) {
		// Answers the PATCH 200 but keeps the old password.
		frame := &fakeFrame{password: "old", acceptButIgnore: true}
		svc, db, id := setupFramePasswordTest(t, frame, "old")

		_, err := svc.ChangeFramePassword(id, "new", "")
		var fpe *FramePasswordError
		require.ErrorAs(t, err, &fpe)
		assert.Equal(t, FrameRefused, fpe.Kind)
		assert.Equal(t, "old", storedPassword(t, db, id))
	})
	t.Run("auth state mismatch", func(t *testing.T) {
		// An open frame that answers 200 but keeps no password: the
		// read-back succeeds (an open frame takes any password), but
		// http_auth_enabled stays false.
		frame := &fakeFrame{acceptButIgnore: true}
		svc, db, id := setupFramePasswordTest(t, frame, "")

		_, err := svc.ChangeFramePassword(id, "new", "")
		var fpe *FramePasswordError
		require.ErrorAs(t, err, &fpe)
		assert.Equal(t, FrameRefused, fpe.Kind)
		assert.Equal(t, "", storedPassword(t, db, id))
	})
}

// The frame answered the PATCH 200 and then went away before the read-back:
// follow the frame's word and store the new password, unverified.
func TestChangeFramePasswordUnansweredReadBackIsUnverified(t *testing.T) {
	frame := &fakeFrame{password: "old", goneAfterPatch: true}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	verified, err := svc.ChangeFramePassword(id, "new", "")
	require.NoError(t, err)
	assert.False(t, verified)
	assert.Equal(t, "new", storedPassword(t, db, id))
}

func TestChangeFramePasswordOldFirmwareIsRefusedUpFront(t *testing.T) {
	frame := &fakeFrame{noAuthKey: true}
	svc, db, id := setupFramePasswordTest(t, frame, "")

	_, err := svc.ChangeFramePassword(id, "new", "")
	var fpe *FramePasswordError
	require.ErrorAs(t, err, &fpe)
	assert.Equal(t, FrameUnsupported, fpe.Kind)
	assert.Empty(t, frame.patchCreds())
	assert.Equal(t, "", storedPassword(t, db, id))
}

func TestChangeFramePasswordLengthLimit(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	_, err := svc.ChangeFramePassword(id, strings.Repeat("x", 64), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 63 bytes")
	assert.Zero(t, frame.configGets.Load(), "must not contact the frame")
	assert.Equal(t, "old", storedPassword(t, db, id))

	_, err = svc.ChangeFramePassword(id, strings.Repeat("x", 63), "")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("x", 63), storedPassword(t, db, id))
}

func TestChangeFramePasswordUnknownDevice(t *testing.T) {
	svc, _, _ := setupFramePasswordTest(t, &fakeFrame{}, "")
	_, err := svc.ChangeFramePassword(9999, "new", "")
	assert.True(t, errors.Is(err, ErrDeviceNotFound))
}

// A request holding the stored password (a config pull or push) finishes
// before a change starts, and one made after sees the new password.
func TestChangeFramePasswordWaitsForInFlightRequests(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	client, release, err := FrameClient(db, id)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := svc.ChangeFramePassword(id, "new", "")
		done <- err
	}()

	// The held client still works with the old password: the change has
	// not reached the frame.
	time.Sleep(100 * time.Millisecond)
	_, err = client.FetchConfig()
	require.NoError(t, err)
	assert.Empty(t, frame.patchCreds())
	release()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("change did not proceed after release")
	}

	client, release, err = FrameClient(db, id)
	require.NoError(t, err)
	defer release()
	_, err = client.FetchConfig()
	require.NoError(t, err, "a client made after the change uses the new password")
}

func TestUpdateDeviceKeepsStoredPassword(t *testing.T) {
	svc, db, id := setupFramePasswordTest(t, &fakeFrame{}, "keep-me")

	d, err := svc.UpdateDevice(id, "Renamed", "frame.local", "landscape", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", d.Name)
	assert.Equal(t, "keep-me", storedPassword(t, db, id))
}

// Writes of the connection details wait for a change in progress, so the
// change cannot store its password over theirs.
func TestConnectionWritesWaitForFramePasswordChange(t *testing.T) {
	svc, _, id := setupFramePasswordTest(t, &fakeFrame{}, "")
	// A host nothing answers at, refused at once: SetHTTPPassword checks the
	// password against whatever host it finds, and a name would be resolved.
	elsewhere := deadHost(t)

	l := frameCredLock(id)
	l.Lock() // stands in for a change in progress
	done := make(chan struct{}, 3)
	go func() { _, _ = svc.SetHTTPPassword(id, "typed"); done <- struct{}{} }()
	go func() { _ = svc.DeleteDevice(id); done <- struct{}{} }()
	go func() {
		_, _ = svc.UpdateDevice(id, "Frame", elsewhere, "", false, false, false, false, 0, 0, "", "", "", "", "", false, "", "")
		done <- struct{}{}
	}()
	select {
	case <-done:
		t.Fatal("a connection write ran during a frame password change")
	case <-time.After(100 * time.Millisecond):
	}
	l.Unlock()
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("connection writes did not proceed after the change")
		}
	}
}

func TestFrameClientAtRefusesAnEditedHost(t *testing.T) {
	_, db, id := setupFramePasswordTest(t, &fakeFrame{}, "pw")
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)

	client, release, err := FrameClientAt(db, id, d.Host)
	require.NoError(t, err)
	release()
	assert.Equal(t, d.Host, client.Host())

	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("host", "other.local").Error)
	_, _, err = FrameClientAt(db, id, d.Host)
	assert.ErrorContains(t, err, "host changed")
}

func TestChangeFramePasswordRefusesAnUnexpectedHost(t *testing.T) {
	frame := &fakeFrame{password: "old"}
	svc, db, id := setupFramePasswordTest(t, frame, "old")

	_, err := svc.ChangeFramePassword(id, "new", "some-other-frame.local")
	assert.ErrorIs(t, err, ErrFrameHostChanged)
	assert.Zero(t, frame.configGets.Load(), "must not contact the frame")
	assert.Equal(t, "old", storedPassword(t, db, id))

	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	_, err = svc.ChangeFramePassword(id, "new", d.Host)
	require.NoError(t, err)
}
