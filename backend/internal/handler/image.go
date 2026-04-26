package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"

	_ "image/jpeg"
	_ "image/png"

	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/imageops"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type ImageHandlerDeps struct {
	Settings       *service.SettingsService
	Renderer       *service.RendererService
	Processor      *service.ProcessorService
	Google         *googlephotos.Client
	CalendarGoogle *googlephotos.Client
	Synology       *service.SynologyService
	Immich         *service.ImmichService
	AIGen          *service.AIGenerationService
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
	google         *googlephotos.Client
	calendarGoogle *googlephotos.Client
	synology       *service.SynologyService
	immich         *service.ImmichService
	aiGen          *service.AIGenerationService
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
		google:         deps.Google,
		calendarGoogle: deps.CalendarGoogle,
		synology:       deps.Synology,
		immich:         deps.Immich,
		aiGen:          deps.AIGen,
		weather:        deps.Weather,
		calendar:       deps.Calendar,
		auth:           deps.Auth,
		db:             deps.DB,
		dataDir:        deps.DataDir,
	}
}

func (h *ImageHandler) ServeImage(c echo.Context) error {
	// Get source from route parameter
	source := c.Param("source")

	// 1. Identify Device and Determine Settings
	// Three-tier identification: token DeviceID → X-Hostname → client IP
	var device model.Device
	deviceFound := false

	// Tier 1: Token-based identification (works over internet)
	if devID, ok := c.Get("device_id").(uint); ok && devID > 0 {
		if err := h.db.First(&device, devID).Error; err == nil {
			deviceFound = true
		}
	}

	// Tier 2: X-Hostname header (backward compat, LAN setups)
	if !deviceFound {
		if hostname := c.Request().Header.Get("X-Hostname"); hostname != "" {
			if err := h.db.Where("host = ?", hostname).First(&device).Error; err == nil {
				deviceFound = true
			}
		}
	}

	// Tier 3: Client IP (backward compat, LAN setups)
	if !deviceFound {
		clientIP := c.RealIP()
		if err := h.db.Where("host = ?", clientIP).First(&device).Error; err == nil {
			deviceFound = true
		}
	}

	// Native resolution of the device panel
	nativeW, nativeH := 800, 480
	// Logical resolution for image generation (respects orientation)
	logicalW, logicalH := 800, 480

	enableCollage := false
	showDate := false
	showPhotoDate := false
	showWeather := false
	var lat, lon float64

	if deviceFound {
		nativeW = device.Width
		nativeH = device.Height
		logicalW, logicalH = nativeW, nativeH

		enableCollage = device.EnableCollage
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

	var servedImageIDs []uint // Track which IDs were served (1 or 2 if collage)

	if source == model.SourceAIGeneration {
		// AI Generation: generate fresh image from device config
		if !deviceFound {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "device not found - AI generation requires device config"})
		}
		img, err = h.aiGen.Generate(&device)
	} else if enableCollage {
		var devID *uint
		if deviceFound {
			devID = &device.ID
		}
		img, servedImageIDs, err = h.fetchSmartCollage(logicalW, logicalH, source, excludeIDs, devID)
	} else {
		var id uint
		var devID *uint
		if deviceFound {
			devID = &device.ID
		}
		img, id, err = h.fetchRandomPhoto(source, excludeIDs, devID)
		if err == nil {
			servedImageIDs = append(servedImageIDs, id)
		}
	}

	if err != nil {
		if strings.Contains(err.Error(), "invalid source filter") {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "invalid source"})
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found") {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "no photos found for this device"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch photo: " + err.Error()})
	}

	// Look up PhotoTakenAt from the first served image
	if deviceFound && device.ShowPhotoDate && len(servedImageIDs) > 0 && servedImageIDs[0] != 0 {
		var servedImg model.Image
		if dbErr := h.db.Select("photo_taken_at").First(&servedImg, servedImageIDs[0]).Error; dbErr == nil {
			photoTakenAt = servedImg.PhotoTakenAt
		}
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
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "render failed: " + renderErr.Error()})
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

	headerOpts := h.processor.MapProcessingSettings(settings, palette)
	for k, v := range headerOpts {
		procOptions[k] = v
	}

	log.Println("Processing image with options: ", procOptions)
	processedBytes, thumbBytes, err := h.processor.ProcessImage(imgWithOverlay, procOptions)
	if err != nil {
		fmt.Printf("Processor failed: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "processor service failed: " + err.Error()})
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
	if deviceFound && device.ConfigLastUpdated > 0 {
		deviceConfigTS := int64(0)
		if tsStr := c.Request().Header.Get("X-Config-Last-Updated"); tsStr != "" {
			if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
				deviceConfigTS = ts
			}
		}
		if device.ConfigLastUpdated > deviceConfigTS {
			payload := buildConfigPayload(&device)
			if payload != "" {
				c.Response().Header().Set("X-Config-Payload", payload)
				log.Printf("Config sync: pushing config to device (server=%d, device=%d)",
					device.ConfigLastUpdated, deviceConfigTS)
			}
		}
	}

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
	// Identify device using same logic as ServeImage
	var device model.Device
	deviceFound := false

	if devID, ok := c.Get("device_id").(uint); ok && devID > 0 {
		if err := h.db.First(&device, devID).Error; err == nil {
			deviceFound = true
		}
	}
	if !deviceFound {
		if hostname := c.Request().Header.Get("X-Hostname"); hostname != "" {
			if err := h.db.Where("host = ?", hostname).First(&device).Error; err == nil {
				deviceFound = true
			}
		}
	}
	if !deviceFound {
		clientIP := c.RealIP()
		if err := h.db.Where("host = ?", clientIP).First(&device).Error; err == nil {
			deviceFound = true
		}
	}

	if !deviceFound {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "device not found"})
	}

	// Parse request body: { "config": {...}, "processing_settings": {...}, "color_palette": {...}, "config_last_updated": 123 }
	var req struct {
		Config             json.RawMessage `json:"config"`
		ProcessingSettings json.RawMessage `json:"processing_settings"`
		ColorPalette       json.RawMessage `json:"color_palette"`
		ConfigLastUpdated  int64           `json:"config_last_updated"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
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
		return c.JSON(http.StatusNotFound, map[string]string{"error": "device not found"})
	}

	var req struct {
		Config             json.RawMessage `json:"config"`
		ProcessingSettings json.RawMessage `json:"processing_settings"`
		ColorPalette       json.RawMessage `json:"color_palette"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
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
		if imageURL, ok := configMap["image_url"].(string); ok && strings.Contains(imageURL, "/image/") {
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
		return c.JSON(http.StatusNotFound, map[string]string{"error": "device not found"})
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	thumbPath := filepath.Join(h.dataDir, fmt.Sprintf("thumb_%s.jpg", id))
	data, err := os.ReadFile(thumbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "thumbnail not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read thumbnail"})
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

// Helper to retrieve settings safely
func (h *ImageHandler) getOrientation() string {
	val, err := h.settings.Get("orientation")
	if err != nil || val == "" {
		return "landscape"
	}
	return val
}

// fetchSmartCollage fetches one or two photos and creates a collage if the
// first photo's orientation doesn't match the device orientation.
func (h *ImageHandler) fetchSmartCollage(screenW, screenH int, sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, []uint, error) {
	devicePortrait := screenH > screenW

	img1, id1, err := h.fetchRandomPhoto(sourceFilter, excludeIDs, deviceID)
	if err != nil {
		return nil, nil, err
	}
	servedIDs := []uint{id1}

	bounds := img1.Bounds()
	isPhotoPortrait := bounds.Dy() > bounds.Dx()

	// Orientation matches - no collage needed
	if isPhotoPortrait == devicePortrait {
		return img1, servedIDs, nil
	}

	// Orientation mismatch - try to find a second photo for collage
	var targetType string
	if devicePortrait {
		targetType = "landscape"
	} else {
		targetType = "portrait"
	}

	excludeWithHistory := append(append([]uint(nil), excludeIDs...), id1)

	// 1. Try with full exclusions (history + id1)
	img2, id2, err := h.fetchRandomPhotoWithType(targetType, sourceFilter, excludeWithHistory, deviceID)
	if err != nil || id2 == id1 {
		log.Printf("SmartCollage: query with history exclusion failed for %s: %v, retrying without history", targetType, err)
		// 2. Try with only id1 excluded (ignore history)
		img2, id2, err = h.fetchRandomPhotoWithType(targetType, sourceFilter, []uint{id1}, deviceID)
	}

	if err == nil && id2 != id1 {
		servedIDs = append(servedIDs, id2)
	} else {
		log.Printf("SmartCollage: no different %s photo found, using same photo twice", targetType)
		img2 = img1
		servedIDs = append(servedIDs, id1)
	}

	if devicePortrait {
		return h.createVerticalCollage(img1, img2, screenW, screenH), servedIDs, nil
	}
	return h.createHorizontalCollage(img1, img2, screenW, screenH), servedIDs, nil
}

// fetchRandomPhotoWithType fetches a random photo matching the given orientation.
// orientations "auto" is always included as a match.
func (h *ImageHandler) fetchRandomPhotoWithType(targetType string, sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, uint, error) {
	query := h.db.Order("RANDOM()").Where("orientation IN ?", []string{targetType, "auto"})

	if len(excludeIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeIDs)
	}

	query, earlyResult, err := h.applySourceFilter(query, sourceFilter, deviceID)
	if earlyResult != nil || err != nil {
		return earlyResult, 0, err
	}

	var item model.Image
	if err := query.First(&item).Error; err != nil {
		return nil, 0, err
	}

	img, err := h.loadImageFromRecord(item)
	if err != nil {
		return nil, 0, err
	}
	return img, item.ID, nil
}

func (h *ImageHandler) createVerticalCollage(img1, img2 image.Image, width, height int) image.Image {
	// Target Dimension: width x height (Portrait)
	// Each slot: width x (height/2)
	slotHeight := height / 2

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw Top
	imageops.DrawCover(dst, image.Rect(0, 0, width, slotHeight), img1)

	// Draw Bottom
	imageops.DrawCover(dst, image.Rect(0, slotHeight, width, height), img2)

	return dst
}

func (h *ImageHandler) createHorizontalCollage(img1, img2 image.Image, width, height int) image.Image {
	// Target Dimension: width x height (Landscape)
	// Each slot: (width/2) x height
	slotWidth := width / 2

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw Left
	imageops.DrawCover(dst, image.Rect(0, 0, slotWidth, height), img1)

	// Draw Right
	imageops.DrawCover(dst, image.Rect(slotWidth, 0, width, height), img2)

	return dst
}

// fetchSynologyPhoto retrieves the photo from Synology Service
func (h *ImageHandler) fetchSynologyPhoto(item model.Image) (image.Image, uint, error) {
	// Try fetching cache first? Or direct from Service which handles fetching
	data, err := h.synology.GetPhoto(item.SynologyPhotoID, item.ThumbnailKey, "large")
	if err != nil {
		return nil, 0, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	return img, item.ID, nil
}

// resolvePath handles path differences between Docker (/data/...) and local dev
func (h *ImageHandler) resolvePath(path string) string {
	// 1. If path exists as is, return it
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// 2. If path starts with /data/, try replacing it with h.dataDir
	// Docker uses /data, local uses whatever DATA_DIR is (e.g. ./data)
	if strings.HasPrefix(path, "/data/") {
		relPath := strings.TrimPrefix(path, "/data/")
		newPath := filepath.Join(h.dataDir, relPath)
		if _, err := os.Stat(newPath); err == nil {
			return newPath
		}
	}

	// 3. Similar check for /app/data/ just in case
	if strings.HasPrefix(path, "/app/data/") {
		relPath := strings.TrimPrefix(path, "/app/data/")
		newPath := filepath.Join(h.dataDir, relPath)
		if _, err := os.Stat(newPath); err == nil {
			return newPath
		}
	}

	return path
}

// fetchRandomPhoto fetches a random photo from the given source, excluding
// the given IDs. Falls back to ignoring exclusions if no match is found.
func (h *ImageHandler) fetchRandomPhoto(sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, uint, error) {
	query := h.db.Order("RANDOM()")

	if len(excludeIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeIDs)
	}

	query, earlyResult, err := h.applySourceFilter(query, sourceFilter, deviceID)
	if earlyResult != nil || err != nil {
		return earlyResult, 0, err
	}

	var item model.Image
	if err := query.First(&item).Error; err != nil {
		if len(excludeIDs) == 0 {
			return nil, 0, err
		}
		// Retry without exclusions
		retryQuery := h.db.Order("RANDOM()")
		retryQuery, earlyResult, retryErr := h.applySourceFilter(retryQuery, sourceFilter, deviceID)
		if earlyResult != nil || retryErr != nil {
			return earlyResult, 0, retryErr
		}
		if err := retryQuery.First(&item).Error; err != nil {
			return nil, 0, err
		}
	}

	img, err := h.loadImageFromRecord(item)
	if err != nil {
		log.Printf("Warning: Failed to load image id=%d: %v", item.ID, err)
		return nil, 0, err
	}
	return img, item.ID, nil
}

// applySourceFilter adds source-specific WHERE clauses to the query.
// For URL proxy sources, it fetches the image directly and returns it as
// earlyResult (the caller should return immediately).
func (h *ImageHandler) applySourceFilter(query *gorm.DB, sourceFilter string, deviceID *uint) (*gorm.DB, image.Image, error) {
	switch sourceFilter {
	case model.SourceGooglePhotos, model.SourceSynologyPhotos, model.SourceGallery, model.SourceImmich:
		return query.Where("source = ?", sourceFilter), nil, nil
	case model.SourceURLProxy:
		img, _, err := h.fetchRandomURLProxy(deviceID)
		return nil, img, err
	default:
		return nil, nil, fmt.Errorf("invalid source filter: %s", sourceFilter)
	}
}

// fetchRandomURLProxy picks a random URL source for the device and fetches it.
func (h *ImageHandler) fetchRandomURLProxy(deviceID *uint) (image.Image, uint, error) {
	var urlSource model.URLSource
	subQuery := h.db.Table("url_sources").Select("url_sources.id, url_sources.url")
	if deviceID != nil {
		subQuery = subQuery.Joins("LEFT JOIN device_url_mappings ON url_sources.id = device_url_mappings.url_source_id").
			Where("device_url_mappings.device_id = ? OR device_url_mappings.device_id IS NULL", *deviceID)
	} else {
		subQuery = subQuery.Joins("LEFT JOIN device_url_mappings ON url_sources.id = device_url_mappings.url_source_id").
			Where("device_url_mappings.device_id IS NULL")
	}
	if err := subQuery.Order("RANDOM()").Limit(1).Scan(&urlSource).Error; err != nil {
		return nil, 0, err
	}
	if urlSource.URL == "" {
		return nil, 0, gorm.ErrRecordNotFound
	}
	return h.fetchURLPhoto(urlSource.URL)
}

// fetchImmichPhoto retrieves the photo from Immich Service
func (h *ImageHandler) fetchImmichPhoto(item model.Image) (image.Image, uint, error) {
	data, err := h.immich.DownloadPhoto(item.ImmichAssetID)
	if err != nil {
		return nil, 0, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	return img, item.ID, nil
}

// loadImageFromRecord loads an image from a database record, handling both
// local files and Synology/Immich photos.
func (h *ImageHandler) loadImageFromRecord(item model.Image) (image.Image, error) {
	if item.Source == model.SourceSynologyPhotos {
		img, _, err := h.fetchSynologyPhoto(item)
		return img, err
	}

	if item.Source == model.SourceImmich {
		img, _, err := h.fetchImmichPhoto(item)
		return img, err
	}

	resolvedPath := h.resolvePath(item.FilePath)
	f, err := os.Open(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s (resolved: %s): %w", item.FilePath, resolvedPath, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

func (h *ImageHandler) fetchURLPhoto(url string) (image.Image, uint, error) {
	// Fetch Image from URL
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Failed to fetch URL photo: %v\n", err)
		return nil, 0, err
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		fmt.Printf("Failed to decode URL photo: %v\n", err)
		return nil, 0, err
	}
	// Return 0 as ID for URL sources
	return img, 0, nil
}
