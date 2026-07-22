package handler

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"io/ioutil"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type DeviceHandler struct {
	deviceService   *service.DeviceService
	synologyService *service.SynologyService
	immichService   *service.ImmichService
	immichCache     *service.ImmichCacheService
	db              *gorm.DB
}

func NewDeviceHandler(deviceService *service.DeviceService, synologyService *service.SynologyService, immichService *service.ImmichService, immichCache *service.ImmichCacheService, db *gorm.DB) *DeviceHandler {
	return &DeviceHandler{
		deviceService:   deviceService,
		synologyService: synologyService,
		immichService:   immichService,
		immichCache:     immichCache,
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
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	if req.Host == "" {
		return respondError(c, http.StatusBadRequest, "host required")
	}

	if req.Layout == "" {
		req.Layout = model.LayoutPhotoOverlay
	}

	device, err := h.deviceService.AddDevice(req.Host, req.EnableCollage, req.ShowDate, req.ShowPhotoDate, req.ShowWeather, req.WeatherLat, req.WeatherLon, req.Layout, req.DisplayMode, req.ShowCalendar, req.CalendarID, req.DateFormat)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, device)
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
			// Try local cache first — works even when Immich is offline.
			if h.immichCache != nil && h.immichCache.Enabled() {
				if cached := h.immichCache.Lookup(img.ID); cached != "" {
					// Use the cached file directly
					imagePath = cached
				}
			}

			// Fall back to Immich download if not cached
			if imagePath == "" {
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
			}
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
	// instead of a COUNT per album. When cached_only=true, count only images
	// that have a local cache entry.
	type albumCount struct {
		AlbumID uint
		N       int
	}
	var counts []albumCount
	countQuery := h.db.Model(&model.ImageAlbumMembership{}).
		Select("image_album_memberships.album_id as album_id, COUNT(*) as n").
		Joins("JOIN images ON images.id = image_album_memberships.image_id AND images.deleted_at IS NULL")
	if c.QueryParam("cached_only") == "true" {
		countQuery = countQuery.
			Joins("JOIN immich_caches c ON c.image_id = images.id")
	}
	countQuery.Group("image_album_memberships.album_id").Scan(&counts)
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
