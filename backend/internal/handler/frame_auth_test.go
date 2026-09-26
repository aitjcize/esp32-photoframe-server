package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
)

// gatedFrame answers every path behind the firmware's password gate.
type gatedFrame struct {
	mu       sync.Mutex
	password string
	hits     []string
}

func (f *gatedFrame) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits = append(f.hits, r.Method+" "+r.URL.Path)
		if _, pass, _ := r.BasicAuth(); f.password != "" && pass != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"authentication required"}`)
			return
		}
		switch r.URL.Path {
		case "/api/system-info":
			fmt.Fprint(w, `{"device_name":"Frame","width":800,"height":480,"version":"2.7.0"}`)
		case "/api/config":
			if r.Method == http.MethodGet {
				fmt.Fprintf(w, `{"device_name":"Frame","http_auth_enabled":%v}`, f.password != "")
				return
			}
			fmt.Fprint(w, `{"status":"success"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	})
}

func (f *gatedFrame) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

func deviceRow(t *testing.T, db *gorm.DB, id uint) model.Device {
	t.Helper()
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	return d
}

func flagDevice(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).
		Updates(map[string]interface{}{"auth_required": true, "auth_failed_at": time.Now()}).Error)
}

// waitFlagged polls for the flag a background pull sets.
func waitFlagged(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if deviceRow(t, db, id).AuthRequired {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("device was not flagged as requiring a password")
}

func TestPullDeviceConfigMarksDeviceOnRejectedPassword(t *testing.T) {
	frame := &gatedFrame{password: "actual"}
	srv := httptest.NewServer(frame.handler())
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")

	h := &ImageHandler{db: db}
	h.pullDeviceConfigAsync(deviceRow(t, db, id), time.Now().Unix())
	waitFlagged(t, db, id)
}

func TestPushDeviceConfigReportsRejectedPassword(t *testing.T) {
	frame := &gatedFrame{password: "actual"}
	srv := httptest.NewServer(frame.handler())
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")
	h := &ImageHandler{db: db}

	got := h.pushDeviceConfig(deviceRow(t, db, id), json.RawMessage(`{"scaleMode":"cover"}`), map[string]interface{}{"device_name": "Frame"})
	assert.Equal(t, "auth_required", got)
	assert.True(t, deviceRow(t, db, id).AuthRequired)
	// The first refusal settled it: the config push was not tried.
	assert.Equal(t, []string{"POST /api/settings/processing"}, frame.requests())

	// The password was entered meanwhile: the next push clears the flag.
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("http_password", "actual").Error)
	got = h.pushDeviceConfig(deviceRow(t, db, id), nil, map[string]interface{}{"device_name": "Frame"})
	assert.Equal(t, "synced", got)
	d := deviceRow(t, db, id)
	assert.False(t, d.AuthRequired)
	assert.Nil(t, d.AuthFailedAt)
}

func TestPushDeviceConfigOfflineIsNotAuthRequired(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	db, id := frameAt(t, dead, "stale")
	h := &ImageHandler{db: db}

	got := h.pushDeviceConfig(deviceRow(t, db, id), nil, map[string]interface{}{"device_name": "Frame"})
	assert.Equal(t, "offline", got)
	assert.False(t, deviceRow(t, db, id).AuthRequired)
}

func callDeviceHandler(t *testing.T, db *gorm.DB, id uint, method, body string, fn func(*DeviceHandler, echo.Context) error) *httptest.ResponseRecorder {
	t.Helper()
	h := NewDeviceHandler(service.NewDeviceService(service.DeviceServiceDeps{DB: db}), nil, nil, db)
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprint(id))
	_ = fn(h, c)
	return rec
}

func setHTTPPassword(t *testing.T, db *gorm.DB, id uint, password string) *httptest.ResponseRecorder {
	t.Helper()
	return callDeviceHandler(t, db, id, http.MethodPut, fmt.Sprintf(`{"http_password":%q}`, password),
		(*DeviceHandler).SetHTTPPassword)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body
}

func TestSetHTTPPasswordVerifiesAgainstTheFrame(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		srv := httptest.NewServer((&gatedFrame{password: "actual"}).handler())
		defer srv.Close()
		db, id := frameAt(t, srv, "stale")
		flagDevice(t, db, id)

		rec := setHTTPPassword(t, db, id, "actual")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		body := decode(t, rec)
		assert.Equal(t, true, body["http_password_set"])
		assert.Equal(t, true, body["verified"])
		assert.Equal(t, false, body["auth_required"])
		assert.Nil(t, body["auth_failed_at"])
		assert.NotContains(t, body, "warning")
		d := deviceRow(t, db, id)
		assert.Equal(t, "actual", d.HTTPPassword)
		assert.False(t, d.AuthRequired)
	})

	// A rejected password answers 409 with the code the webapp keys on --
	// never the frame's 401, which the webapp reads as its own session
	// expiring.
	t.Run("rejected", func(t *testing.T) {
		srv := httptest.NewServer((&gatedFrame{password: "actual"}).handler())
		defer srv.Close()
		db, id := frameAt(t, srv, "stale")

		rec := setHTTPPassword(t, db, id, "wrong")
		assert.Equal(t, http.StatusConflict, rec.Code)
		body := decode(t, rec)
		assert.Equal(t, frameAuthRequiredCode, body["code"])
		assert.Contains(t, body["error"], "not stored")
		assert.Equal(t, "stale", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("unreachable", func(t *testing.T) {
		dead := httptest.NewServer(http.NotFoundHandler())
		dead.Close()
		db, id := frameAt(t, dead, "stale")
		flagDevice(t, db, id)

		rec := setHTTPPassword(t, db, id, "typed")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		body := decode(t, rec)
		assert.Equal(t, true, body["http_password_set"])
		assert.Equal(t, false, body["verified"])
		assert.Equal(t, false, body["auth_required"])
		assert.Contains(t, body["warning"], "next wake")
		assert.Equal(t, "typed", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("forgotten while the frame still asks", func(t *testing.T) {
		srv := httptest.NewServer((&gatedFrame{password: "actual"}).handler())
		defer srv.Close()
		db, id := frameAt(t, srv, "actual")

		rec := setHTTPPassword(t, db, id, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		body := decode(t, rec)
		assert.Equal(t, false, body["http_password_set"])
		assert.Equal(t, false, body["verified"])
		assert.Equal(t, true, body["auth_required"])
		assert.NotNil(t, body["auth_failed_at"])
		assert.Contains(t, body["warning"], "still requires a password")
		assert.Equal(t, "", deviceRow(t, db, id).HTTPPassword)
	})

	t.Run("validation", func(t *testing.T) {
		srv := httptest.NewServer((&gatedFrame{}).handler())
		defer srv.Close()
		db, id := frameAt(t, srv, "keep")
		for name, pw := range map[string]string{
			"too long": strings.Repeat("x", 64),
			"NUL":      "abc\x00def",
		} {
			rec := setHTTPPassword(t, db, id, pw)
			assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		}
		assert.Equal(t, "keep", deviceRow(t, db, id).HTTPPassword)
		rec := setHTTPPassword(t, db, 9999, "x")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestPushToDeviceRejectedPasswordIs409(t *testing.T) {
	frame := &gatedFrame{password: "actual"}
	srv := httptest.NewServer(frame.handler())
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")
	img := filepath.Join(t.TempDir(), "photo.jpg")
	require.NoError(t, os.WriteFile(img, []byte("not really a jpeg"), 0o644))

	rec := callDeviceHandler(t, db, id, http.MethodPost, fmt.Sprintf(`{"url":%q}`, img),
		(*DeviceHandler).PushToDevice)
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	body := decode(t, rec)
	assert.Equal(t, frameAuthRequiredCode, body["code"])
	assert.Contains(t, body["error"], "push the image")
	assert.True(t, deviceRow(t, db, id).AuthRequired)
	assert.Equal(t, []string{"GET /api/system-info"}, frame.requests(), "settled before rendering")
}

func TestRefreshDeviceRejectedPasswordIs409(t *testing.T) {
	srv := httptest.NewServer((&gatedFrame{password: "actual"}).handler())
	defer srv.Close()
	db, id := frameAt(t, srv, "stale")

	rec := callDeviceHandler(t, db, id, http.MethodPost, "", (*DeviceHandler).RefreshDevice)
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	body := decode(t, rec)
	assert.Equal(t, frameAuthRequiredCode, body["code"])
	assert.True(t, deviceRow(t, db, id).AuthRequired)
}

func TestRefreshDeviceLockoutIs429WithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "actual")

	rec := callDeviceHandler(t, db, id, http.MethodPost, "", (*DeviceHandler).RefreshDevice)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "30", rec.Header().Get("Retry-After"))
	assert.NotContains(t, rec.Body.String(), frameAuthRequiredCode)
	assert.False(t, deviceRow(t, db, id).AuthRequired, "a lockout is not a refusal")
}

// The list the webapp draws its badges from carries the flag.
func TestListDevicesReportsAuthRequired(t *testing.T) {
	srv := httptest.NewServer((&gatedFrame{}).handler())
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	flagDevice(t, db, id)

	rec := callDeviceHandler(t, db, id, http.MethodGet, "", (*DeviceHandler).ListDevices)
	require.Equal(t, http.StatusOK, rec.Code)
	var devices []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &devices))
	require.Len(t, devices, 1)
	assert.Equal(t, true, devices[0]["auth_required"])
	assert.NotEmpty(t, devices[0]["auth_failed_at"])
	assert.NotContains(t, devices[0], "http_password")
}
