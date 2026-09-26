package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"io/ioutil"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DeviceHandler struct {
	deviceService   *service.DeviceService
	synologyService *service.SynologyService
	immichService   *service.ImmichService
	db              *gorm.DB
}

func NewDeviceHandler(deviceService *service.DeviceService, synologyService *service.SynologyService, immichService *service.ImmichService, db *gorm.DB) *DeviceHandler {
	return &DeviceHandler{
		deviceService:   deviceService,
		synologyService: synologyService,
		immichService:   immichService,
		db:              db,
	}
}

// ... existing methods ... (List, Add, Update, Delete, Push)

// GET /api/devices
func (h *DeviceHandler) ListDevices(c echo.Context) error {
	devices, err := h.deviceService.ListDevices()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, devices)
}

// POST /api/devices
func (h *DeviceHandler) AddDevice(c echo.Context) error {
	var req struct {
		Host          string  `json:"host"`
		EnableCollage bool    `json:"enable_collage"`
		ShowDate      bool    `json:"show_date"`
		ShowPhotoDate bool    `json:"show_photo_date"`
		ShowWeather   bool    `json:"show_weather"`
		WeatherLat    float64 `json:"weather_lat"`
		WeatherLon    float64 `json:"weather_lon"`
		Layout        string  `json:"layout"`
		DisplayMode   string  `json:"display_mode"`
		ShowCalendar  bool    `json:"show_calendar"`
		CalendarID    string  `json:"calendar_id"`
		DateFormat    string  `json:"date_format"`
		// Only needed when the frame requires a password on its own HTTP API.
		HTTPPassword string `json:"http_password"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if req.Host == "" {
		return respondError(c, http.StatusBadRequest, "host required")
	}

	if len(req.HTTPPassword) > photoframe.MaxHTTPPasswordLen {
		return respondError(c, http.StatusBadRequest, fmt.Sprintf("frame password must be at most %d bytes", photoframe.MaxHTTPPasswordLen))
	}

	if req.Layout == "" {
		req.Layout = model.LayoutPhotoOverlay
	}

	device, err := h.deviceService.AddDevice(req.Host, req.HTTPPassword, req.EnableCollage, req.ShowDate, req.ShowPhotoDate, req.ShowWeather, req.WeatherLat, req.WeatherLon, req.Layout, req.DisplayMode, req.ShowCalendar, req.CalendarID, req.DateFormat)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, device)
}

// frameAuthRequiredCode marks a 409 that stands for the frame's 401 -- the
// frame refused the password this server has for it -- so the webapp can
// tell it from other conflicts and ask for the frame's password. It is a 409
// rather than the frame's 401 because the webapp reads a 401 as its own
// session expiring and logs the user out.
const frameAuthRequiredCode = "frame_auth_required"

// respondFrameAuthRequired answers that the frame refused the stored
// password, as {"error": msg, "code": "frame_auth_required"} with a 409.
func respondFrameAuthRequired(c echo.Context, msg string) error {
	if err := c.JSON(http.StatusConflict, map[string]string{"error": msg, "code": frameAuthRequiredCode}); err != nil {
		return err
	}
	return errors.New(msg)
}

// respondFrameAuthError answers for an err that is the frame refusing the
// stored password (401 -> 409 with frameAuthRequiredCode) or refusing to
// check it for now (429, with its Retry-After), and reports whether it did.
// Any other error is left to the caller. what names the action, for the
// message ("push the image").
func respondFrameAuthError(c echo.Context, err error, what string) (bool, error) {
	var se *photoframe.StatusError
	if !errors.As(err, &se) {
		return false, nil
	}
	switch se.StatusCode {
	case http.StatusUnauthorized:
		return true, respondFrameAuthRequired(c,
			"The frame refused the password this server has for it, so it could not "+what+". "+
				"Enter the frame's current password for this device and try again.")
	case http.StatusTooManyRequests:
		msg := "The frame is refusing password attempts from this server after too many wrong ones, so it could not " + what + "."
		if se.RetryAfter != "" {
			c.Response().Header().Set("Retry-After", se.RetryAfter)
			msg += " Try again in " + se.RetryAfter + " seconds."
		} else {
			msg += " Try again later."
		}
		return true, respondError(c, http.StatusTooManyRequests, msg)
	}
	return false, nil
}

// SetHTTPPassword stores the password for a frame whose own HTTP API is
// password-protected (esp32-photoframe #130), after checking it against the
// frame. Write-only on purpose: the value is never returned, only the
// http_password_set flag on the device. Send an empty string to forget it,
// which is what you do after turning the frame's authentication back off.
//
// A password the frame rejects is answered 409 (frameAuthRequiredCode) and
// not stored. One the frame cannot confirm -- it is asleep, locked out, or
// not asking for a password right now -- is stored anyway, with verified
// false and a warning saying why, since the user may be entering it ahead
// of the frame's next wake.
//
// PUT /api/devices/:id/http-password
func (h *DeviceHandler) SetHTTPPassword(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid device id")
	}
	var req struct {
		HTTPPassword string `json:"http_password"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if err := photoframe.ValidateHTTPPassword(req.HTTPPassword); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	unverified, err := h.deviceService.SetHTTPPassword(uint(id), req.HTTPPassword)
	if err != nil {
		var fpe *service.FramePasswordError
		switch {
		case errors.Is(err, service.ErrDeviceNotFound):
			return respondError(c, http.StatusNotFound, "device not found")
		case errors.As(err, &fpe) && fpe.Kind == service.FrameWrongPassword:
			return respondFrameAuthRequired(c,
				"The frame rejected this password, so it was not stored. "+
					"Check the password on the frame's own web page and try again.")
		}
		return respondError(c, http.StatusInternalServerError, err.Error())
	}

	var device model.Device
	if err := h.db.Select("id", "auth_required", "auth_failed_at").First(&device, id).Error; err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	resp := map[string]interface{}{
		"http_password_set": req.HTTPPassword != "",
		"verified":          unverified == nil,
		"auth_required":     device.AuthRequired,
		"auth_failed_at":    device.AuthFailedAt,
	}
	if unverified != nil {
		resp["warning"] = httpPasswordUnverifiedMessage(unverified, req.HTTPPassword != "")
	}
	return c.JSON(http.StatusOK, resp)
}

// httpPasswordUnverifiedMessage says why the frame could not confirm a
// password that was stored anyway, and what happens next.
func httpPasswordUnverifiedMessage(e *service.FramePasswordError, set bool) string {
	stored := "The password was stored and will be used on the frame's next wake."
	if !set {
		stored = "The stored password was forgotten."
	}
	switch e.Kind {
	case service.FrameNoHost:
		return "This device has no host, so the frame could not be asked. " + stored
	case service.FrameWrongPassword:
		// Only for a forgotten password: a rejected one is not stored.
		return "The frame still requires a password, and this server now has none for it. " + stored
	case service.FrameLockedOut:
		msg := "The frame is refusing password checks from this server after too many wrong ones"
		if e.RetryAfter != "" {
			msg += " (for another " + e.RetryAfter + " seconds)"
		}
		return msg + ", so this one could not be checked. " + stored
	case service.FrameOpen:
		return "The frame does not require a password at the moment, so it could not confirm this one. " + stored
	case service.FrameUnsupported:
		return "The frame's firmware does not support a password, so it could not confirm this one. " + stored
	case service.FrameRefused:
		return "The frame answered, but did not confirm the password: " + e.Err.Error() + ". " + stored
	default:
		return "The frame could not be reached, so the password could not be checked. " + stored
	}
}

// ChangeFramePassword changes the password on the frame itself -- unlike
// SetHTTPPassword, which only records one the frame already has -- and then
// stores it so the server keeps talking to the frame. An empty password turns
// the frame's password off. If the frame does not take it, the stored
// password is left as it was.
//
// Frame-side failures never answer 401: the webapp reads a 401 as its own
// session expiring and logs the user out.
//
// POST /api/devices/:id/frame-password
func (h *DeviceHandler) ChangeFramePassword(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid device id")
	}
	var req struct {
		Password *string `json:"password"`
		// The host the user saw when asking; optional.
		Host string `json:"host"`
	}
	if err := c.Bind(&req); err != nil || req.Password == nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if err := photoframe.ValidateHTTPPassword(*req.Password); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}

	verified, err := h.deviceService.ChangeFramePassword(uint(id), *req.Password, req.Host)
	if err != nil {
		var fpe *service.FramePasswordError
		var fse *service.FramePasswordStoreError
		switch {
		case errors.Is(err, service.ErrDeviceNotFound):
			return respondError(c, http.StatusNotFound, "device not found")
		case errors.Is(err, service.ErrFrameHostChanged):
			return respondError(c, http.StatusConflict,
				"This device's host was changed elsewhere since the dialog was opened. Nothing was changed; reopen the device and try again.")
		case errors.As(err, &fpe):
			status, msg := framePasswordErrorResponse(fpe)
			if fpe.Kind == service.FrameLockedOut && fpe.RetryAfter != "" {
				c.Response().Header().Set("Retry-After", fpe.RetryAfter)
			}
			return respondError(c, status, msg)
		case errors.As(err, &fse):
			return respondError(c, http.StatusInternalServerError,
				"The frame now uses the new password, but the server could not save it ("+fse.Err.Error()+"). "+
					"Enter the new password under \"Frame password\" and save, or the server cannot reach the frame.")
		}
		return respondError(c, http.StatusInternalServerError, err.Error())
	}

	// The store cleared the refusal flag along with the old password (see
	// service.ChangeFramePassword); reported as SetHTTPPassword reports it,
	// so the webapp can drop its badge without reloading the list.
	var device model.Device
	if err := h.db.Select("id", "auth_required", "auth_failed_at").First(&device, id).Error; err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	resp := map[string]interface{}{
		"http_password_set": *req.Password != "",
		"verified":          verified,
		"auth_required":     device.AuthRequired,
		"auth_failed_at":    device.AuthFailedAt,
	}
	if !verified {
		resp["warning"] = "The frame accepted the change, but reading its settings back with the new password did not confirm it. " +
			"The server now uses the new password; check the frame from its own web page."
	}
	return c.JSON(http.StatusOK, resp)
}

// framePasswordErrorResponse turns a refused frame password change into a
// status and a message the webapp shows as is. Every message says what the
// stored password is now, because that is what the user most needs to know.
func framePasswordErrorResponse(e *service.FramePasswordError) (int, string) {
	const unchanged = " Nothing was changed."
	switch e.Kind {
	case service.FrameNoHost:
		return http.StatusBadRequest, "This device has no host, so the server cannot reach the frame." + unchanged
	case service.FrameWrongPassword:
		// 409, not 401: see ChangeFramePassword.
		msg := "The frame rejected the password this server has for it, so it would not accept a new one."
		if e.NoneStored {
			msg = "The frame already requires a password, and this server has none stored for it."
		}
		return http.StatusConflict, msg + unchanged +
			" Enter the frame's current password under \"Frame password\" and save, then try again."
	case service.FrameLockedOut:
		msg := "The frame is refusing password attempts from this server after too many wrong ones."
		if e.RetryAfter != "" {
			msg += " Try again in " + e.RetryAfter + " seconds."
		} else {
			msg += " Try again later."
		}
		return http.StatusTooManyRequests, msg + unchanged
	case service.FrameRefused:
		return http.StatusBadGateway, "The frame refused the change: " + e.Err.Error() + "." + unchanged
	case service.FrameOutcomeUnknown:
		// Not "Nothing was changed": that may be false here.
		return http.StatusGatewayTimeout, "The frame stopped answering during the change, so it is not known whether it now has the new password. " +
			"The server still uses the old one. If the frame no longer responds to the server, enter the new password under \"Frame password\" and save."
	case service.FrameUnsupported:
		return http.StatusBadGateway, "The frame's firmware does not support a password. Update the firmware first." + unchanged
	default:
		return http.StatusBadGateway, "Could not reach the frame: " + e.Err.Error() + "." + unchanged
	}
}

// PUT /api/devices/:id
// Updates server-owned + shared fields only. Dimensions / board name
// come from POST /api/devices/:id/refresh.
func (h *DeviceHandler) UpdateDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name            string  `json:"name"`
		Host            string  `json:"host"`
		Orientation     string  `json:"orientation"`
		EnableCollage   bool    `json:"enable_collage"`
		ShowDate        bool    `json:"show_date"`
		ShowPhotoDate   bool    `json:"show_photo_date"`
		ShowWeather     bool    `json:"show_weather"`
		WeatherLat      float64 `json:"weather_lat"`
		WeatherLon      float64 `json:"weather_lon"`
		AIProvider      string  `json:"ai_provider"`
		AIModel         string  `json:"ai_model"`
		AIPrompt        string  `json:"ai_prompt"`
		Layout          string  `json:"layout"`
		DisplayMode     string  `json:"display_mode"`
		BackgroundColor string  `json:"background_color"`
		ShowCalendar    bool    `json:"show_calendar"`
		CalendarID      string  `json:"calendar_id"`
		DateFormat      string  `json:"date_format"`
		Source          string  `json:"source"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if req.Layout == "" {
		req.Layout = model.LayoutPhotoOverlay
	}

	device, err := h.deviceService.UpdateDevice(uint(id), req.Name, req.Host, req.Orientation, req.EnableCollage, req.ShowDate, req.ShowPhotoDate, req.ShowWeather, req.WeatherLat, req.WeatherLon, req.AIProvider, req.AIModel, req.AIPrompt, req.Layout, req.DisplayMode, req.ShowCalendar, req.CalendarID, req.DateFormat)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	// Per-device image source + fit-mode background color for the unified
	// /image endpoint (kept out of the long UpdateDevice signature; persisted
	// directly).
	if err := h.db.Model(device).Updates(map[string]interface{}{
		"source":           req.Source,
		"background_color": req.BackgroundColor,
	}).Error; err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	device.Source = req.Source
	device.BackgroundColor = req.BackgroundColor
	return c.JSON(http.StatusOK, device)
}

// POST /api/devices/:id/refresh
// Pulls dimensions, board name, config, processing settings, and palette
// from the device. Requires the device to be online.
func (h *DeviceHandler) RefreshDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	device, err := h.deviceService.RefreshDeviceFromHardware(uint(id))
	if err != nil {
		if done, rerr := respondFrameAuthError(c, err, "read the device's settings"); done {
			return rerr
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "failed to fetch") {
			return respondError(c, http.StatusServiceUnavailable, errMsg)
		}
		return respondError(c, http.StatusInternalServerError, errMsg)
	}
	return c.JSON(http.StatusOK, device)
}

// DELETE /api/devices/:id
func (h *DeviceHandler) DeleteDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.deviceService.DeleteDevice(uint(id)); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// POST /api/devices/:id/push
func (h *DeviceHandler) PushToDevice(c echo.Context) error {
	deviceID, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		ImageID uint   `json:"image_id"`
		URL     string `json:"url"` // Optional direct URL/Path
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	imagePath := req.URL
	var tempFile string // If we create a temp file, we must clean it up

	if req.ImageID != 0 {
		var img model.Image
		if err := h.db.First(&img, req.ImageID).Error; err != nil {
			return respondError(c, http.StatusNotFound, "image not found")
		}

		if img.Source == model.SourceSynologyPhotos {
			// The Synology photo id lives in ExternalID, matching the serving path.
			synoID, err := strconv.Atoi(img.ExternalID)
			if err != nil {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("invalid synology photo id %q: %v", img.ExternalID, err))
			}
			// Download to temporary file
			data, err := h.synologyService.DownloadPhoto(synoID)
			if err != nil {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to download synology photo: %v", err))
			}

			// Save to temp file
			tmp, err := ioutil.TempFile("", "syno_push_*.jpg")
			if err != nil {
				return respondError(c, http.StatusInternalServerError, "failed to create temp file")
			}
			defer os.Remove(tmp.Name()) // Clean up
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return respondError(c, http.StatusInternalServerError, "failed to write temp file")
			}
			tmp.Close()
			imagePath = tempFile
		} else if img.Source == model.SourceImmich {
			// Download from Immich to temporary file
			data, err := h.immichService.DownloadPhoto(img.ExternalID)
			if err != nil {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to download immich photo: %v", err))
			}

			tmp, err := ioutil.TempFile("", "immich_push_*.jpg")
			if err != nil {
				return respondError(c, http.StatusInternalServerError, "failed to create temp file")
			}
			defer os.Remove(tmp.Name())
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return respondError(c, http.StatusInternalServerError, "failed to write temp file")
			}
			tmp.Close()
			imagePath = tempFile
		} else if strings.HasPrefix(img.FilePath, "http://") || strings.HasPrefix(img.FilePath, "https://") {
			// Topic sources (unsplash, pexels) store a remote image URL in
			// FilePath rather than a local file — fetch it to a temp file.
			resp, err := http.Get(img.FilePath)
			if err != nil {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to download photo: %v", err))
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to download photo: status %d", resp.StatusCode))
			}
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to read photo: %v", err))
			}

			tmp, err := ioutil.TempFile("", "url_push_*.jpg")
			if err != nil {
				return respondError(c, http.StatusInternalServerError, "failed to create temp file")
			}
			defer os.Remove(tmp.Name())
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return respondError(c, http.StatusInternalServerError, "failed to write temp file")
			}
			tmp.Close()
			imagePath = tempFile
		} else {
			imagePath = img.FilePath
		}
	}

	if imagePath == "" {
		return respondError(c, http.StatusBadRequest, "image path or id required")
	}

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return respondError(c, http.StatusNotFound, "image file not found on server")
	}

	// Push
	if err := h.deviceService.PushToDevice(uint(deviceID), imagePath); err != nil {
		if done, rerr := respondFrameAuthError(c, err, "push the image"); done {
			return rerr
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "not reachable") || strings.Contains(errMsg, "failed to resolve") {
			return respondError(c, http.StatusServiceUnavailable,
				"Device is not reachable. Please ensure the device is online and accessible.")

		}
		return respondError(c, http.StatusInternalServerError, fmt.Sprintf("push failed: %v", err))
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "pushed"})
}

// ListAlbums returns persisted source albums, optionally filtered by
// ?source= and ?synced=true. Feeds the device album picker and gallery chips.
// GET /api/albums
func (h *DeviceHandler) ListAlbums(c echo.Context) error {
	q := h.db.Model(&model.Album{})
	if src := c.QueryParam("source"); src != "" {
		q = q.Where("source = ?", src)
	}
	if c.QueryParam("synced") == "true" {
		q = q.Where("sync_enabled = ?", true)
	}
	var albums []model.Album
	if err := q.Order("name").Find(&albums).Error; err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	// Report the LIVE photo count (memberships joined to existing images) rather
	// than the cached Album.asset_count, which can go stale when images are
	// removed (cleared, or an album emptied/deleted upstream). One grouped query
	// instead of a COUNT per album.
	type albumCount struct {
		AlbumID uint
		N       int
	}
	var counts []albumCount
	h.db.Model(&model.ImageAlbumMembership{}).
		Select("image_album_memberships.album_id as album_id, COUNT(*) as n").
		Joins("JOIN images ON images.id = image_album_memberships.image_id AND images.deleted_at IS NULL").
		Group("image_album_memberships.album_id").
		Scan(&counts)
	countByID := make(map[uint]int, len(counts))
	for _, cnt := range counts {
		countByID[cnt.AlbumID] = cnt.N
	}
	for i := range albums {
		albums[i].AssetCount = countByID[albums[i].ID]
	}
	return c.JSON(http.StatusOK, albums)
}

// GetDeviceAlbums returns the album IDs a device is bound to, optionally
// scoped to ?source=.
// GET /api/devices/:id/albums
func (h *DeviceHandler) GetDeviceAlbums(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid device id")
	}
	q := h.db.Model(&model.DeviceAlbumMapping{}).
		Joins("JOIN albums ON albums.id = device_album_mappings.album_id").
		Where("device_album_mappings.device_id = ?", id)
	if src := c.QueryParam("source"); src != "" {
		q = q.Where("albums.source = ?", src)
	}
	ids := []uint{}
	if err := q.Pluck("device_album_mappings.album_id", &ids).Error; err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"album_ids": ids})
}

// UpdateDeviceAlbums replaces a device's album bindings for one source.
// PUT /api/devices/:id/albums  body: {source, album_ids:[]}
func (h *DeviceHandler) UpdateDeviceAlbums(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return respondError(c, http.StatusBadRequest, "invalid device id")
	}
	var req struct {
		Source   string `json:"source"`
		AlbumIDs []uint `json:"album_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if req.Source == "" {
		return respondError(c, http.StatusBadRequest, "source required")
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Replace mappings for this device scoped to the given source only.
		sub := tx.Model(&model.Album{}).Select("id").Where("source = ?", req.Source)
		if e := tx.Where("device_id = ? AND album_id IN (?)", id, sub).
			Delete(&model.DeviceAlbumMapping{}).Error; e != nil {
			return e
		}
		for _, aid := range req.AlbumIDs {
			var album model.Album
			if e := tx.Where("id = ? AND source = ?", aid, req.Source).First(&album).Error; e != nil {
				return fmt.Errorf("album %d not found for source %s", aid, req.Source)
			}
			if e := tx.Create(&model.DeviceAlbumMapping{DeviceID: uint(id), AlbumID: aid}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
