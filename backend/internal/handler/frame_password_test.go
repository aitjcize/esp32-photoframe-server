package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
)

var handlerFrameDBCounter atomic.Int64

// frameAt stores a device pointing at srv with the given stored password.
// serialiseTestDB gives the shared-cache in-memory database the single
// connection production runs on (internal/db). With a pool, a write from a
// goroutine and a read from the test land on different connections, and
// shared-cache SQLite answers the collision with "table is locked" rather
// than waiting.
func serialiseTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
}

func frameAt(t *testing.T, srv *httptest.Server, stored string) (*gorm.DB, uint) {
	t.Helper()
	n := handlerFrameDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:handler_frame_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	serialiseTestDB(t, db)
	require.NoError(t, db.AutoMigrate(&model.Device{}))
	d := model.Device{Name: "Frame", Host: strings.TrimPrefix(srv.URL, "http://"), HTTPPassword: stored}
	require.NoError(t, db.Create(&d).Error)
	return db, d.ID
}

func changeFramePassword(t *testing.T, db *gorm.DB, id uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewDeviceHandler(service.NewDeviceService(service.DeviceServiceDeps{DB: db}), nil, nil, db)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprint(id))
	_ = h.ChangeFramePassword(c)
	return rec
}

// passwordFrame answers GET /api/config behind a password gate, and hands
// PATCH to patch.
func passwordFrame(password string, patch http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pass, _ := r.BasicAuth(); pass != password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPatch {
			patch(w, r)
			return
		}
		fmt.Fprintf(w, `{"http_auth_enabled":%v}`, password != "")
	}))
}

func storedFramePassword(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	return d.HTTPPassword
}

// The frame's 401 must not reach the webapp as a 401, which it reads as its
// own session expiring and logs the user out.
func TestChangeFramePasswordFrame401IsNot401(t *testing.T) {
	srv := passwordFrame("actual", nil)
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")

	rec := changeFramePassword(t, db, id, `{"password":"new"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Nothing was changed")
	assert.Equal(t, "stale", storedFramePassword(t, db, id))
}

func TestChangeFramePasswordFrame429PassesRetryAfter(t *testing.T) {
	srv := passwordFrame("old", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	defer srv.Close()
	db, id := frameAt(t, srv, "old")

	rec := changeFramePassword(t, db, id, `{"password":"new"}`)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "30", rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "30 seconds")
	assert.Equal(t, "old", storedFramePassword(t, db, id))
}

func TestChangeFramePasswordRequestValidation(t *testing.T) {
	srv := passwordFrame("old", nil)
	defer srv.Close()
	db, id := frameAt(t, srv, "old")

	for name, body := range map[string]string{
		"too long": fmt.Sprintf(`{"password":%q}`, strings.Repeat("x", 64)),
		// The firmware would keep only "abc".
		"NUL": `{"password":"abc\u0000def"}`,
		// A missing field must not be read as "" and turn protection off.
		"missing": `{}`,
		"null":    `{"password":null}`,
	} {
		rec := changeFramePassword(t, db, id, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
	}
	assert.Equal(t, "old", storedFramePassword(t, db, id))
}

func TestChangeFramePasswordSuccessResponse(t *testing.T) {
	var mu sync.Mutex
	password := "old"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if _, pass, _ := r.BasicAuth(); pass != password {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPatch {
			var body struct {
				HTTPPassword string `json:"http_password"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			password = body.HTTPPassword
			return
		}
		fmt.Fprintf(w, `{"http_auth_enabled":%v}`, password != "")
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "old")

	rec := changeFramePassword(t, db, id, `{"password":""}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"http_password_set":false,"verified":true,"auth_required":false,"auth_failed_at":null}`, rec.Body.String())
	assert.Equal(t, "", storedFramePassword(t, db, id))
}

// A device config edit must never carry the frame password: pushed directly
// it would change the frame's password behind the stored one. Refused before
// anything is stored or pushed.
func TestUpdateDeviceConfigRefusesHTTPPassword(t *testing.T) {
	for name, cfg := range map[string]string{
		"with other fields": `{"device_name":"Renamed","http_password":"sneaky"}`,
		"password only":     `{"http_password":"sneaky"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var pushes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pushes.Add(1)
			}))
			defer srv.Close()
			db, id := frameAt(t, srv, "")
			require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).
				Updates(map[string]interface{}{"device_config": `{"device_name":"Frame"}`, "config_last_updated": 100}).Error)

			h := &ImageHandler{db: db}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"config":`+cfg+`}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(fmt.Sprint(id))
			_ = h.UpdateDeviceConfig(c)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, pushes.Load())
			var d model.Device
			require.NoError(t, db.First(&d, id).Error)
			assert.Equal(t, `{"device_name":"Frame"}`, d.DeviceConfig)
			assert.Equal(t, int64(100), d.ConfigLastUpdated)
		})
	}
}

// A config pull that the frame refuses must stop, not keep guessing: each
// retry would spend another of the frame's free wrong passwords.
func TestPullDeviceConfigStopsOnRejectedPassword(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")

	h := &ImageHandler{db: db}
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	h.pullDeviceConfigAsync(d, time.Now().Unix())

	// The loop retries every 2s; wait past one retry.
	time.Sleep(2500 * time.Millisecond)
	assert.Equal(t, int32(1), hits.Load())
}
