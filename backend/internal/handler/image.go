package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"

	_ "image/jpeg"

	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ImageHandlerDeps is the dependency bundle the handler needs at construction.
// Photo-library backends (synology, immich, google, on-disk gallery) live
// inside their respective imagesource plugins now — the handler itself
// only needs DB access for device lookup / history, the renderer for
// overlays, the processor for dithering, the weather/calendar clients for
// overlay data, the auth service, and the registered source registry.
type ImageHandlerDeps struct {
	Settings       *service.SettingsService
	Renderer       *service.RendererService
	Processor      *service.ProcessorService
	CalendarGoogle *googlephotos.Client
	Sources        *imagesource.Registry
	Weather        *weather.Client
	Calendar       *gcalendar.Client
	Auth           *service.AuthService
	DB             *gorm.DB
	DataDir        string
}

type ImageHandler struct {
	settings       *service.SettingsService
	renderer       *service.RendererService
	processor      *service.ProcessorService
	calendarGoogle *googlephotos.Client
	sources        *imagesource.Registry
	weather        *weather.Client
	calendar       *gcalendar.Client
	auth           *service.AuthService
	db             *gorm.DB
	dataDir        string
}

func NewImageHandler(deps ImageHandlerDeps) *ImageHandler {
	return &ImageHandler{
		settings:       deps.Settings,
		renderer:       deps.Renderer,
		processor:      deps.Processor,
		calendarGoogle: deps.CalendarGoogle,
		sources:        deps.Sources,
		weather:        deps.Weather,
		calendar:       deps.Calendar,
		auth:           deps.Auth,
		db:             deps.DB,
		dataDir:        deps.DataDir,
	}
}

// identifyDevice resolves the requesting device SOLELY from its per-device
// token (the device_id set by the auth middleware). The previous X-Hostname /
// client-IP fallbacks were client-spoofable and have been removed — the token
// is the single source of truth. Returns the device and whether one was found.
func (h *ImageHandler) identifyDevice(c echo.Context) (model.Device, bool) {
	var device model.Device
	if devID, ok := c.Get("device_id").(uint); ok && devID > 0 {
		if err := h.db.First(&device, devID).Error; err == nil {
			return device, true
		}
	}
	return device, false
}

func (h *ImageHandler) ServeImage(c echo.Context) error {
	// Get source from route parameter
	source := c.Param("source")

	// 1. Identify the device from its token (the single source of truth).
	device, deviceFound := h.identifyDevice(c)

	// Resolve the image source. An identified device with a configured source
	// ALWAYS gets that source — the URL path param cannot override it — so a
	// stale or wrong firmware URL can never cause the wrong source/album to be
	// served. The path param is honored only for unconfigured (legacy / not
	// yet migrated) or unidentified requests. Bare /image with neither is 400.
	if deviceFound && device.Source != "" {
		source = device.Source
	} else if source == "" {
		return respondError(c, http.StatusBadRequest,
			"no image source for this device — set the device's Image Source in the server, or request /image/<source>")

	}

	// Native resolution of the device panel
	nativeW, nativeH := 800, 480
	// Logical resolution for image generation (respects orientation)
	logicalW, logicalH := 800, 480

	showDate := false
	showPhotoDate := false
	showWeather := false
	var lat, lon float64

	if deviceFound {
		nativeW = device.Width
		nativeH = device.Height
		logicalW, logicalH = nativeW, nativeH

		showDate = device.ShowDate
		showPhotoDate = device.ShowPhotoDate
		showWeather = device.ShowWeather
		lat = device.WeatherLat
		lon = device.WeatherLon
	}

	// ALWAYS overrides logical resolution/orientation from Headers if present
	if wStr := c.Request().Header.Get("X-Display-Width"); wStr != "" {
		if w, err := strconv.Atoi(wStr); err == nil && w > 0 {
			logicalW = w
			nativeW = w
			if deviceFound && device.Width != w {
				device.Width = w
				h.db.Model(&device).Update("width", w)
			}
		}
	}
	if hStr := c.Request().Header.Get("X-Display-Height"); hStr != "" {
		if he, err := strconv.Atoi(hStr); err == nil && he > 0 {
			logicalH = he
			nativeH = he
			if deviceFound && device.Height != he {
				device.Height = he
				h.db.Model(&device).Update("height", he)
			}
		}
	}
	// Determine effective orientation from header or device config
	orientation := ""
	if oStr := c.Request().Header.Get("X-Display-Orientation"); oStr != "" {
		orientation = oStr
		// Persist orientation update to database if it changed
		if deviceFound && device.Orientation != oStr {
			device.Orientation = oStr
			h.db.Model(&device).Update("orientation", oStr)
		}
	} else if deviceFound {
		orientation = device.Orientation
	}

	// Swap logical dimensions to match orientation (used for overlays and collage)
	if orientation == "portrait" && logicalW > logicalH {
		logicalW, logicalH = logicalH, logicalW
	} else if orientation == "landscape" && logicalW < logicalH {
		logicalW, logicalH = logicalH, logicalW
	}

	layout := model.LayoutPhotoOverlay
	displayMode := "cover"
	showCalendar := false

	if deviceFound {
		if device.Layout != "" {
			layout = device.Layout
		}
		if device.DisplayMode != "" {
			displayMode = device.DisplayMode
		}
		showCalendar = device.ShowCalendar
	}

	var img image.Image
	var err error
	var photoTakenAt *time.Time

	// 1.5. Get Device History for Exclusion
	var excludeIDs []uint
	if deviceFound {
		// History retention: ensure we don't repeat recent 50 images
		// Get last 50 served images for this device
		var history []model.DeviceHistory
		if err := h.db.Where("device_id = ?", device.ID).
			Order("served_at desc").
			Limit(50).
			Find(&history).Error; err == nil {
			for _, h := range history {
				excludeIDs = append(excludeIDs, h.ImageID)
			}
		}
	}

	// All image sources — synthetic (AI, fractal, DLA) and library-backed
	// (gallery, immich, synology, google_photos, url_proxy) — flow through
	// the unified imagesource.Registry.
	if !h.sources.Has(source) {
		return respondError(c, http.StatusNotFound, "invalid source")
	}
	var devicePtr *model.Device
	if deviceFound {
		devicePtr = &device
	}
	sourceResp, err := h.sources.Fetch(source, &imagesource.Request{
		Device:       devicePtr,
		Source:       source,
		Width:        logicalW,
		Height:       logicalH,
		NativeWidth:  nativeW,
		NativeHeight: nativeH,
		Orientation:  orientation,
		ExcludeIDs:   excludeIDs,
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid source filter") {
			return respondError(c, http.StatusNotFound, "invalid source")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found") {
			return respondError(c, http.StatusNotFound, "no photos found for this device")
		}
		return respondError(c, http.StatusInternalServerError, "failed to fetch photo: "+err.Error())
	}
	img = sourceResp.Image
	servedImageIDs := sourceResp.ImageIDs
	if sourceResp.PhotoTakenAt != nil {
		photoTakenAt = sourceResp.PhotoTakenAt
	}

	// If the source asked to bypass post-processing, encode straight to PNG
	// and ship it. The renderer overlay and epaper-image-convert pipeline
	// are skipped — the source already produced a panel-ready image, and
	// CDR / preprocessing would shift its flat color regions.
	if sourceResp.SkipPostProcessing {
		out := img
		if logicalW != nativeW || logicalH != nativeH {
			out = rotate90CW(out)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, out); err != nil {
			return respondError(c, http.StatusInternalServerError, "png encode: "+err.Error())
		}
		body := buf.Bytes()

		applyConfigSyncHeader(c, &device, deviceFound)

		c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		return c.Blob(http.StatusOK, "image/png", body)
	}

	// 1.6. Record History
	if deviceFound && len(servedImageIDs) > 0 {
		go func(devID uint, imgIDs []uint) {
			rows := make([]model.DeviceHistory, 0, len(imgIDs))
			now := time.Now()
			for _, imgID := range imgIDs {
				if imgID == 0 {
					continue
				}
				rows = append(rows, model.DeviceHistory{
					DeviceID: devID,
					ImageID:  imgID,
					ServedAt: now,
				})
			}
			if len(rows) == 0 {
				return
			}
			// Insert + prune in a single transaction so we acquire the
			// SQLite write lock once instead of three times. The prune
			// finds the served_at of the 51st-newest row and range-deletes
			// anything older; both halves hit the
			// idx_device_histories_device_served composite index, so this
			// stays O(log n) instead of the O(n) scan the previous
			// "NOT IN (subquery)" form degraded into.
			h.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
				var cutoffs []time.Time
				if err := tx.Model(&model.DeviceHistory{}).
					Where("device_id = ?", devID).
					Order("served_at desc").
					Offset(50).
					Limit(1).
					Pluck("served_at", &cutoffs).Error; err != nil || len(cutoffs) == 0 {
					return nil
				}
				return tx.Where("device_id = ? AND served_at < ?", devID, cutoffs[0]).
					Delete(&model.DeviceHistory{}).Error
			})
		}(device.ID, servedImageIDs)
	}

	// 2. Render layout (photo + overlay + calendar)
	needsOverlay := showDate || showPhotoDate || showWeather || showCalendar
	var imgWithOverlay image.Image

	if needsOverlay {
		var weatherData *weather.CurrentWeather
		var deviceTimezone string
		if showWeather && lat != 0 && lon != 0 {
			latStr := fmt.Sprintf("%f", lat)
			lonStr := fmt.Sprintf("%f", lon)
			var weatherErr error
			weatherData, weatherErr = h.weather.GetWeather(latStr, lonStr)
			if weatherErr != nil {
				log.Printf("Failed to fetch weather data: %v", weatherErr)
			}
			if weatherData != nil {
				deviceTimezone = weatherData.Timezone
			}
		}

		var events []gcalendar.Event
		if showCalendar && h.calendar != nil && h.calendarGoogle != nil {
			httpClient, err := h.calendarGoogle.GetClient()
			if err == nil {
				calendarID := device.CalendarID
				if calendarID == "" {
					calendarID = "primary"
				}
				var calErr error
				events, calErr = h.calendar.GetTodayEvents(httpClient, calendarID, deviceTimezone)
				if calErr != nil {
					log.Printf("Failed to fetch calendar events: %v", calErr)
				}
			}
		}

		var renderErr error
		imgWithOverlay, renderErr = h.renderer.Render(service.RenderOptions{
			Layout:        layout,
			DisplayMode:   displayMode,
			Width:         logicalW,
			Height:        logicalH,
			NativeWidth:   nativeW,
			NativeHeight:  nativeH,
			Photo:         img,
			ShowDate:      showDate,
			ShowPhotoDate: showPhotoDate,
			PhotoDate:     photoTakenAt,
			ShowWeather:   showWeather,
			Weather:       weatherData,
			ShowCalendar:  showCalendar,
			Events:        events,
			Timezone:      deviceTimezone,
			DateFormat:    device.DateFormat,
		})
		if renderErr != nil {
			return respondError(c, http.StatusInternalServerError, "render failed: "+renderErr.Error())
		}
	} else {
		imgWithOverlay = img
	}

	// 3. Tone Mapping + Thumbnail (CLI)
	// Always pass native panel dimensions. The CLI handles orientation
	// internally (swaps dims, processes, rotates output to native layout).
	procOptions := map[string]string{
		"dimension": fmt.Sprintf("%dx%d", nativeW, nativeH),
	}
	if orientation != "" {
		procOptions["orientation"] = orientation
	}

	// Determine output format based on firmware version (epdgz requires >= 2.6.1)
	firmwareVersion := c.Request().Header.Get("X-Firmware-Version")
	if firmwareVersion == "" || !photoframe.SupportsEPDGZ(firmwareVersion) {
		procOptions["format"] = "png"
	}

	// 3.5. Parse X-Processing-Settings header if present
	var settings *photoframe.ProcessingSettings
	if settingsStr := c.Request().Header.Get("X-Processing-Settings"); settingsStr != "" {
		settings = &photoframe.ProcessingSettings{}
		if err := json.Unmarshal([]byte(settingsStr), settings); err != nil {
			fmt.Printf("Failed to parse X-Processing-Settings header: %v\n", err)
			settings = nil
		}
	}

	// 3.6. Parse X-Color-Palette header if present
	var palette *photoframe.Palette
	if paletteStr := c.Request().Header.Get("X-Color-Palette"); paletteStr != "" {
		palette = &photoframe.Palette{}
		if err := json.Unmarshal([]byte(paletteStr), palette); err != nil {
			fmt.Printf("Failed to parse X-Color-Palette header: %v\n", err)
			palette = nil
		}
	}

	// GC16 grayscale mode: prefer the device-reported display type, falling back
	// to the board name for firmware that predates the display_type field.
	// Only treat as grayscale when the device is known.
	grayscale := deviceFound && device.IsGrayscale()

	headerOpts := h.processor.MapProcessingSettings(settings, palette, grayscale)
	for k, v := range headerOpts {
		procOptions[k] = v
	}

	log.Println("Processing image with options: ", procOptions)
	processedBytes, thumbBytes, err := h.processor.ProcessImage(imgWithOverlay, procOptions)
	if err != nil {
		fmt.Printf("Processor failed: %v\n", err)
		return respondError(c, http.StatusInternalServerError, "processor service failed: "+err.Error())
	}

	// 4. Cache Thumbnail & Set Headers
	if thumbBytes != nil {
		thumbID := fmt.Sprintf("%d", time.Now().UnixNano())
		thumbPath := filepath.Join(h.dataDir, fmt.Sprintf("thumb_%s.jpg", thumbID))

		if err := os.WriteFile(thumbPath, thumbBytes, 0644); err == nil {
			thumbnailUrl := fmt.Sprintf("http://%s/served-image-thumbnail/%s", c.Request().Host, thumbID)
			c.Response().Header().Set("X-Thumbnail-URL", thumbnailUrl)
		} else {
			fmt.Printf("Failed to save served thumbnail: %v\n", err)
		}
	}

	// 5. Config Sync: push config payload if server has newer config
	applyConfigSyncHeader(c, &device, deviceFound)

	// Set Content-Length header
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(processedBytes)))

	contentType := "application/octet-stream"
	if firmwareVersion == "" || !photoframe.SupportsEPDGZ(firmwareVersion) {
		contentType = "image/png"
	}
	return c.Blob(http.StatusOK, contentType, processedBytes)
}

// SyncDeviceConfig handles device config sync.
// The device POSTs its current config; the server stores it and returns its own
// config if it's newer.
// POST /api/device-config/sync
func (h *ImageHandler) SyncDeviceConfig(c echo.Context) error {
	device, deviceFound := h.identifyDevice(c)
	if !deviceFound {
		return respondError(c, http.StatusNotFound, "device not found")
	}

	// Parse request body: { "config": {...}, "processing_settings": {...}, "color_palette": {...}, "config_last_updated": 123 }
	var req struct {
		Config             json.RawMessage `json:"config"`
		ProcessingSettings json.RawMessage `json:"processing_settings"`
		ColorPalette       json.RawMessage `json:"color_palette"`
		ConfigLastUpdated  int64           `json:"config_last_updated"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	// Store device's config in database
	updates := map[string]interface{}{}
	if len(req.Config) > 0 {
		updates["device_config"] = string(req.Config)
	}
	if len(req.ProcessingSettings) > 0 {
		updates["device_processing_settings"] = string(req.ProcessingSettings)
	}
	if len(req.ColorPalette) > 0 {
		updates["device_color_palette"] = string(req.ColorPalette)
	}
	if req.ConfigLastUpdated > 0 {
		updates["config_last_updated"] = req.ConfigLastUpdated
	}

	if len(updates) > 0 {
		h.db.Model(&device).Updates(updates)
	}

	// Return server's config if it's newer
	resp := map[string]interface{}{
		"status":              "synced",
		"config_last_updated": device.ConfigLastUpdated,
	}

	return c.JSON(http.StatusOK, resp)
}

// UpdateDeviceConfig updates the server-side device config (called from web UI).
// PUT /api/devices/:id/config
func (h *ImageHandler) UpdateDeviceConfig(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	var device model.Device
	if err := h.db.First(&device, uint(id)).Error; err != nil {
		return respondError(c, http.StatusNotFound, "device not found")
	}

	var req struct {
		Config             json.RawMessage `json:"config"`
		ProcessingSettings json.RawMessage `json:"processing_settings"`
		ColorPalette       json.RawMessage `json:"color_palette"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}

	updates := map[string]interface{}{
		"config_last_updated": time.Now().Unix(),
	}
	if len(req.Config) > 0 {
		updates["device_config"] = string(req.Config)
	}
	if len(req.ProcessingSettings) > 0 {
		updates["device_processing_settings"] = string(req.ProcessingSettings)
	}
	if len(req.ColorPalette) > 0 {
		updates["device_color_palette"] = string(req.ColorPalette)
	}

	h.db.Model(&device).Updates(updates)

	// If image_url points to this server, ensure a device token is included
	var configMap map[string]interface{}
	if len(req.Config) > 0 {
		json.Unmarshal(req.Config, &configMap)
	}
	if configMap != nil {
		// Matches the unified bare "/image", the legacy "/image/<source>",
		// and any "/image?..." query form.
		if imageURL, ok := configMap["image_url"].(string); ok &&
			(strings.Contains(imageURL, "/image/") ||
				strings.HasSuffix(imageURL, "/image") ||
				strings.Contains(imageURL, "/image?")) {
			// Generate or reuse a device token
			if userID, ok := c.Get("user_id").(uint); ok {
				username, _ := c.Get("username").(string)
				token, err := h.auth.GetOrGenerateDeviceToken(userID, username, device.Name, &device.ID)
				if err == nil {
					configMap["access_token"] = token
					// Re-serialize with token for DB storage
					updated, _ := json.Marshal(configMap)
					updates["device_config"] = string(updated)
					h.db.Model(&device).Update("device_config", string(updated))
				}
			}
		}
	}

	// Attempt to push config to device directly
	pushResult := "synced"
	if device.Host != "" && configMap != nil {
		if err := photoframe.NewClient(device.Host).PushConfig(configMap); err != nil {
			log.Printf("Could not push config to device %s: %v (will sync on next image fetch)", device.Host, err)
			pushResult = "offline"
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":              "updated",
		"push_result":         pushResult,
		"config_last_updated": updates["config_last_updated"],
	})
}

// GetDeviceConfig returns the server-side device config.
// GET /api/devices/:id/config
func (h *ImageHandler) GetDeviceConfig(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))

	var device model.Device
	if err := h.db.First(&device, uint(id)).Error; err != nil {
		return respondError(c, http.StatusNotFound, "device not found")
	}

	resp := map[string]interface{}{
		"config_last_updated": device.ConfigLastUpdated,
	}

	if device.DeviceConfig != "" && device.DeviceConfig != "{}" {
		resp["config"] = json.RawMessage(device.DeviceConfig)
	}
	if device.DeviceProcessingSettings != "" && device.DeviceProcessingSettings != "{}" {
		resp["processing_settings"] = json.RawMessage(device.DeviceProcessingSettings)
	}
	if device.DeviceColorPalette != "" && device.DeviceColorPalette != "{}" {
		resp["color_palette"] = json.RawMessage(device.DeviceColorPalette)
	}

	return c.JSON(http.StatusOK, resp)
}

// buildConfigPayload builds the X-Config-Payload JSON from device's stored config.
// Returns empty string if there's nothing to send.
func buildConfigPayload(device *model.Device) string {
	payload := map[string]json.RawMessage{}

	if device.DeviceConfig != "" && device.DeviceConfig != "{}" {
		payload["config"] = json.RawMessage(device.DeviceConfig)
	}
	if device.DeviceProcessingSettings != "" && device.DeviceProcessingSettings != "{}" {
		payload["processing_settings"] = json.RawMessage(device.DeviceProcessingSettings)
	}
	if device.DeviceColorPalette != "" && device.DeviceColorPalette != "{}" {
		payload["color_palette"] = json.RawMessage(device.DeviceColorPalette)
	}

	if len(payload) == 0 {
		return ""
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func (h *ImageHandler) GetServedImageThumbnail(c echo.Context) error {
	id := c.Param("id")
	// Prevent directory traversal
	if id == "" || id == "." || id == ".." {
		return respondError(c, http.StatusBadRequest, "invalid id")
	}

	thumbPath := filepath.Join(h.dataDir, fmt.Sprintf("thumb_%s.jpg", id))
	data, err := os.ReadFile(thumbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return respondError(c, http.StatusNotFound, "thumbnail not found")
		}
		return respondError(c, http.StatusInternalServerError, "failed to read thumbnail")
	}

	// Delete after 5 minutes instead of immediately
	go func() {
		time.Sleep(5 * time.Minute)
		os.Remove(thumbPath)
	}()

	// Set Content-Length header
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	return c.Blob(http.StatusOK, "image/jpeg", data)
}

// applyConfigSyncHeader sets the X-Config-Payload response header when the
// server's stored device config is newer than what the device most recently
// reported. Pulled out so both the bypass branch and the main flow share it.
func applyConfigSyncHeader(c echo.Context, device *model.Device, deviceFound bool) {
	if !deviceFound || device.ConfigLastUpdated <= 0 {
		return
	}
	deviceConfigTS := int64(0)
	if tsStr := c.Request().Header.Get("X-Config-Last-Updated"); tsStr != "" {
		if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
			deviceConfigTS = ts
		}
	}
	if device.ConfigLastUpdated <= deviceConfigTS {
		return
	}
	payload := buildConfigPayload(device)
	if payload == "" {
		return
	}
	c.Response().Header().Set("X-Config-Payload", payload)
	log.Printf("Config sync: pushing config to device (server=%d, device=%d)",
		device.ConfigLastUpdated, deviceConfigTS)
}

// rotate90CW returns src rotated 90° clockwise. Used for bypass sources to
// translate from the device's logical (oriented) layout to the panel's
// native physical layout.
func rotate90CW(src image.Image) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, sh, sw))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			dst.Set(sh-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
