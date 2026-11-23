package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
)

func putDeviceConfig(t *testing.T, h *ImageHandler, id uint, body string) map[string]interface{} {
	t.Helper()
	return putDeviceConfigAs(t, h, id, body, 0)
}

// putDeviceConfigAs saves as a signed-in user (userID > 0), the way the
// dashboard does, which is what gets a server image URL its device token.
func putDeviceConfigAs(t *testing.T, h *ImageHandler, id uint, body string, userID uint) map[string]interface{} {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID > 0 {
		c.Set("user_id", userID)
		c.Set("username", "admin")
	}
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprint(id))
	require.NoError(t, h.UpdateDeviceConfig(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// A save that changes only server-side settings sends the frame's settings
// unchanged. The frame is not contacted (an asleep one would hold the save up
// until the connection times out), and nothing is stored, so the sync
// timestamp stays where it was.
func TestUpdateDeviceConfigSkipsUnchangedSettings(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	// The stored copy is pulled whole from the frame, so it holds fields the
	// dialog never sends.
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config":              `{"device_name":"Frame","auto_rotate":true,"rotate_cron":"0 * * * *","firmware_only":1}`,
		"device_processing_settings": `{"exposure":1.2,"scaleMode":"cover"}`,
		"device_color_palette":       `{"black":{"r":0,"g":0,"b":0}}`,
		"config_last_updated":        100,
	}).Error)

	h := &ImageHandler{db: db}
	resp := putDeviceConfig(t, h, id, `{
		"config":{"rotate_cron":"0 * * * *","auto_rotate":true,"device_name":"Frame"},
		"processing_settings":{"scaleMode":"cover","exposure":1.2},
		"color_palette":{"black":{"b":0,"g":0,"r":0}}}`)

	assert.Equal(t, "unchanged", resp["push_result"])
	assert.EqualValues(t, 100, resp["config_last_updated"])
	assert.Zero(t, hits.Load())
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	assert.Equal(t, int64(100), d.ConfigLastUpdated)
	assert.Contains(t, d.DeviceConfig, "firmware_only")
}

// Any field the frame holds that differs, or that the stored copy lacks,
// is stored and pushed as before.
func TestUpdateDeviceConfigPushesChangedSettings(t *testing.T) {
	for name, body := range map[string]string{
		"config value":        `{"config":{"device_name":"Renamed"}}`,
		"new config field":    `{"config":{"device_name":"Frame","timezone":"UTC"}}`,
		"processing settings": `{"config":{"device_name":"Frame"},"processing_settings":{"exposure":1.5}}`,
		"color palette":       `{"config":{"device_name":"Frame"},"color_palette":{"black":{"r":1,"g":0,"b":0}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var configPushes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/config" {
					configPushes.Add(1)
				}
				w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			db, id := frameAt(t, srv, "")
			require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
				"device_config":              `{"device_name":"Frame"}`,
				"device_processing_settings": `{"exposure":1.2}`,
				"device_color_palette":       `{"black":{"r":0,"g":0,"b":0}}`,
				"config_last_updated":        100,
			}).Error)

			h := &ImageHandler{db: db}
			resp := putDeviceConfig(t, h, id, body)

			assert.Equal(t, "synced", resp["push_result"])
			assert.Equal(t, int32(1), configPushes.Load())
			var d model.Device
			require.NoError(t, db.First(&d, id).Error)
			assert.Greater(t, d.ConfigLastUpdated, int64(100))
		})
	}
}

// countingFrame answers every request, failing them while fail is set, and
// counts the config pushes it gets.
func countingFrame(fail *atomic.Bool, pushes *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/config" {
			pushes.Add(1)
		}
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{}`))
	}))
}

func configUndelivered(t *testing.T, h *ImageHandler, id uint) bool {
	t.Helper()
	var d model.Device
	require.NoError(t, h.db.First(&d, id).Error)
	return d.ConfigUndelivered
}

// Settings whose push failed are retried when saved again unchanged, which is
// how a user delivers them once the frame is awake; the frame may never fetch
// from this server to pick them up otherwise. Once delivered, unchanged saves
// leave the frame alone again.
func TestUpdateDeviceConfigRetriesUndeliveredSettings(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "config_last_updated": 100,
	}).Error)
	h := &ImageHandler{db: db}
	edit := `{"config":{"device_name":"Renamed"}}`

	fail.Store(true)
	assert.Equal(t, "offline", putDeviceConfig(t, h, id, edit)["push_result"])
	assert.True(t, configUndelivered(t, h, id))
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	stamped := d.ConfigLastUpdated

	fail.Store(false)
	resp := putDeviceConfig(t, h, id, edit)
	assert.Equal(t, "synced", resp["push_result"])
	assert.EqualValues(t, stamped, resp["config_last_updated"], "a retry is not a new edit")
	assert.False(t, configUndelivered(t, h, id))
	assert.Equal(t, int32(2), pushes.Load())

	assert.Equal(t, "unchanged", putDeviceConfig(t, h, id, edit)["push_result"])
	assert.Equal(t, int32(2), pushes.Load())
}

// A frame that reports the stored settings' timestamp has them, so a later
// unchanged save need not push them again.
func TestApplyConfigSyncMarksSettingsDelivered(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "config_last_updated": 100, "config_undelivered": true,
	}).Error)
	h := &ImageHandler{db: db}
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Config-Last-Updated", "100")
	c := echo.New().NewContext(req, httptest.NewRecorder())
	h.applyConfigSync(c, &d, true)

	assert.False(t, configUndelivered(t, h, id))
	assert.Equal(t, "unchanged", putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"}}`)["push_result"])
	assert.Zero(t, pushes.Load())
}

// A server image URL carries the frame's device token. Re-saving keeps the
// stored token while it is valid, rather than signing a fresh one that would
// make every save look like a change.
func TestUpdateDeviceConfigKeepsTheFramesToken(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.AutoMigrate(&model.APIKey{}))
	auth := service.NewAuthService(db, "test-secret", false)
	h := &ImageHandler{db: db, auth: auth}
	edit := `{"config":{"device_name":"Frame","image_url":"http://server:9607/image"}}`

	assert.Equal(t, "synced", putDeviceConfigAs(t, h, id, edit, 1)["push_result"])
	var stored struct {
		AccessToken string `json:"access_token"`
	}
	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	require.NoError(t, json.Unmarshal([]byte(d.DeviceConfig), &stored))
	require.NotEmpty(t, stored.AccessToken)

	// A fresh token would carry a later IssuedAt
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, "unchanged", putDeviceConfigAs(t, h, id, edit, 1)["push_result"])
	assert.Equal(t, int32(1), pushes.Load())

	// A token from another secret is not kept
	require.NoError(t, auth.RotateSecret(func(string) error { return nil }))
	assert.Equal(t, "synced", putDeviceConfigAs(t, h, id, edit, 1)["push_result"])
	assert.Equal(t, int32(2), pushes.Load())
}

// The dialog shows a grayscale frame's calibration rounded to three decimals,
// while the frame reports its float32 values; untouched, they are the same.
func TestUpdateDeviceConfigToleratesDialogRounding(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config":        `{"device_name":"Frame"}`,
		"device_color_palette": `{"black_y":0.008999999612569809,"white_y":0.6499999761581421,"gamma":1.4199999570846558}`,
		"config_last_updated":  100,
	}).Error)
	h := &ImageHandler{db: db}

	resp := putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"},
		"color_palette":{"black_y":0.009,"white_y":0.65,"gamma":1.42}}`)
	assert.Equal(t, "unchanged", resp["push_result"])

	resp = putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"},
		"color_palette":{"black_y":0.009,"white_y":0.651,"gamma":1.42}}`)
	assert.Equal(t, "synced", resp["push_result"])
	assert.Equal(t, int32(1), pushes.Load())
}

// Only a push the frame answers marks the settings delivered. A save that
// pushes nothing (no config in it, or no host to push to) leaves a pending
// edit pending, and an edit that could not be pushed at all is pending too.
func TestUpdateDeviceConfigKeepsPendingWithoutAPush(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "device_processing_settings": `{"exposure":1.2}`,
		"config_last_updated": 100, "config_undelivered": true,
	}).Error)
	h := &ImageHandler{db: db}

	putDeviceConfig(t, h, id, `{"processing_settings":{"exposure":1.5}}`)
	assert.True(t, configUndelivered(t, h, id), "nothing was pushed")
	assert.Zero(t, pushes.Load())

	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"host": "", "config_undelivered": false,
	}).Error)
	putDeviceConfig(t, h, id, `{"config":{"device_name":"Renamed"}}`)
	assert.True(t, configUndelivered(t, h, id), "no host to push to")
	assert.Equal(t, "unchanged", putDeviceConfig(t, h, id, `{"config":{"device_name":"Renamed"}}`)["push_result"])
	assert.Zero(t, pushes.Load())
}

// A frame reporting older settings than the stored ones does not confirm the
// newer edit saved while its request was being served.
func TestApplyConfigSyncKeepsANewerEditPending(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "config_last_updated": 100, "config_undelivered": true,
	}).Error)
	h := &ImageHandler{db: db}
	var snapshot model.Device
	require.NoError(t, db.First(&snapshot, id).Error)
	// Saved while the frame's request was in flight
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Update("config_last_updated", 200).Error)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Config-Last-Updated", "100")
	h.applyConfigSync(echo.New().NewContext(req, httptest.NewRecorder()), &snapshot, true)

	assert.True(t, configUndelivered(t, h, id))
}

// A push confirms only the settings it carried: a different edit stored
// within the same second stays pending.
func TestUpdateDeviceConfigPushConfirmsOnlyItsOwnSettings(t *testing.T) {
	var pushes atomic.Int32
	var db *gorm.DB
	var id uint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/config" && pushes.Add(1) == 1 {
			// Another save lands while this push is in flight
			db.Model(&model.Device{}).Where("id = ?", id).Update("device_config", `{"device_name":"Other"}`)
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	db, id = frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "config_last_updated": 100,
	}).Error)
	h := &ImageHandler{db: db}

	assert.Equal(t, "synced", putDeviceConfig(t, h, id, `{"config":{"device_name":"Renamed"}}`)["push_result"])
	assert.True(t, configUndelivered(t, h, id))
}

// A retry replays everything pending, so a save that carries only part of a
// failed edit still delivers the rest.
func TestUpdateDeviceConfigRetryReplaysAllPendingSettings(t *testing.T) {
	var fail atomic.Bool
	var processingPushes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/settings/processing" {
			processingPushes.Add(1)
		}
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "device_processing_settings": `{"exposure":1.2}`,
		"config_last_updated": 100,
	}).Error)
	h := &ImageHandler{db: db}

	fail.Store(true)
	putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"},"processing_settings":{"exposure":1.5}}`)
	require.True(t, configUndelivered(t, h, id))

	fail.Store(false)
	before := processingPushes.Load()
	assert.Equal(t, "synced", putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"}}`)["push_result"])
	assert.Equal(t, before+1, processingPushes.Load(), "the pending processing settings were replayed")
	assert.False(t, configUndelivered(t, h, id))
}

// A new edit also carries whatever an earlier failed push left pending.
func TestUpdateDeviceConfigNewEditCarriesPendingSettings(t *testing.T) {
	var fail atomic.Bool
	var processing atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/settings/processing" {
			var body bytes.Buffer
			body.ReadFrom(r.Body)
			processing.Store(body.String())
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "device_processing_settings": `{"exposure":1.2}`,
		"config_last_updated": 100,
	}).Error)
	h := &ImageHandler{db: db}

	fail.Store(true)
	putDeviceConfig(t, h, id, `{"config":{"device_name":"Frame"},"processing_settings":{"exposure":1.5}}`)
	fail.Store(false)
	assert.Equal(t, "synced", putDeviceConfig(t, h, id, `{"config":{"device_name":"Renamed"}}`)["push_result"])
	assert.JSONEq(t, `{"exposure":1.5}`, processing.Load().(string))
	assert.False(t, configUndelivered(t, h, id))
}

// Two edits saved within one second get distinct versions, so a frame
// reporting the first does not confirm the second.
func TestUpdateDeviceConfigVersionsEveryEdit(t *testing.T) {
	var fail atomic.Bool
	var pushes atomic.Int32
	srv := countingFrame(&fail, &pushes)
	defer srv.Close()
	db, id := frameAt(t, srv, "")
	future := time.Now().Unix() + 3600
	require.NoError(t, db.Model(&model.Device{}).Where("id = ?", id).Updates(map[string]interface{}{
		"device_config": `{"device_name":"Frame"}`, "config_last_updated": future,
	}).Error)
	h := &ImageHandler{db: db}

	fail.Store(true)
	resp := putDeviceConfig(t, h, id, `{"config":{"device_name":"Renamed"}}`)
	assert.EqualValues(t, future+1, resp["config_last_updated"])

	var d model.Device
	require.NoError(t, db.First(&d, id).Error)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Config-Last-Updated", fmt.Sprint(future))
	h.applyConfigSync(echo.New().NewContext(req, httptest.NewRecorder()), &d, true)
	assert.True(t, configUndelivered(t, h, id))
}
