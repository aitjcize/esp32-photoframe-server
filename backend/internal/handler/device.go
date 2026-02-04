package handler

import (
	"fmt"
	"log"
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
	authService     *service.AuthService
	settingsService *service.SettingsService
	db              *gorm.DB
}

func NewDeviceHandler(deviceService *service.DeviceService, synologyService *service.SynologyService, authService *service.AuthService, settingsService *service.SettingsService, db *gorm.DB) *DeviceHandler {
	return &DeviceHandler{
		deviceService:   deviceService,
		synologyService: synologyService,
		authService:     authService,
		settingsService: settingsService,
		db:              db,
	}
}

// ... existing methods ... (List, Add, Update, Delete, Push)

// POST /api/devices/:id/configure-source
func (h *DeviceHandler) ConfigureDeviceSource(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	deviceID := uint(id)

	var req struct {
		Source string `json:"source"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Source == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "source required"})
	}

	// 1. Determine Image URL
	// For device access, always use the direct add-on port, not the ingress URL
	// ESP32 devices access the server directly, not through Home Assistant ingress
	hostname := c.Request().Host
	// Strip port if present (e.g., homeassistant.local:8123 -> homeassistant.local)
	if idx := strings.Index(hostname, ":"); idx != -1 {
		hostname = hostname[:idx]
	}
	// Use direct add-on port from environment variable (defaults to 9607)
	addonPort := os.Getenv("ADDON_PORT")
	if addonPort == "" {
		addonPort = "9607"
	}
	host := fmt.Sprintf("%s:%s", hostname, addonPort)

	var imageURL string
	switch req.Source {
	case model.SourceURLProxy:
		imageURL = fmt.Sprintf("http://%s/image/url_proxy", host)
	case model.SourceGooglePhotos:
		imageURL = fmt.Sprintf("http://%s/image/google_photos", host)
	case model.SourceSynologyPhotos:
		imageURL = fmt.Sprintf("http://%s/image/synology_photos", host)
	case model.SourceTelegram: // Added telegram source
		imageURL = fmt.Sprintf("http://%s/image/telegram", host)
		// Update Telegram Settings (Append if not exists)
		existingIDs, _ := h.settingsService.Get("telegram_target_device_id")
		newID := fmt.Sprintf("%d", deviceID)

		// Check duplicates
		ids := strings.Split(existingIDs, ",")
		found := false
		for _, id := range ids {
			if strings.TrimSpace(id) == newID {
				found = true
				break
			}
		}

		if !found {
			if existingIDs != "" {
				existingIDs += "," + newID
			} else {
				existingIDs = newID
			}
			if err := h.settingsService.Set("telegram_target_device_id", existingIDs); err != nil {
				log.Printf("Failed to set telegram_target_device_id: %v", err)
			}
		}

		if err := h.settingsService.Set("telegram_push_enabled", "true"); err != nil {
			log.Printf("Failed to set telegram_push_enabled: %v", err)
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid source"})
	}

	configUpdate := map[string]interface{}{
		"image_url":     imageURL,
		"rotation_mode": "url",
		"auto_rotate":   true,
	}

	// Generate Token
	userID, ok := c.Get("user_id").(uint)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
	username := c.Get("username").(string)

	// Get Device Name for Token Name
	var device model.Device
	if err := h.db.First(&device, deviceID).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "device not found"})
	}

	token, err := h.authService.GetOrGenerateDeviceToken(userID, username, device.Name)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token: " + err.Error()})
	}
	configUpdate["access_token"] = token

	// Push Config
	if err := h.deviceService.ConfigureDevice(deviceID, configUpdate); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to push config: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "configured", "url": imageURL})
}

// GET /api/devices
func (h *DeviceHandler) ListDevices(c echo.Context) error {
	devices, err := h.deviceService.ListDevices()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, devices)
}

// POST /api/devices
func (h *DeviceHandler) AddDevice(c echo.Context) error {
	var req struct {
		Host               string  `json:"host"`
		UseDeviceParameter bool    `json:"use_device_parameter"`
		EnableCollage      bool    `json:"enable_collage"`
		ShowDate           bool    `json:"show_date"`
		ShowWeather        bool    `json:"show_weather"`
		WeatherLat         float64 `json:"weather_lat"`
		WeatherLon         float64 `json:"weather_lon"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.Host == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "host required"})
	}

	device, err := h.deviceService.AddDevice(req.Host, req.UseDeviceParameter, req.EnableCollage, req.ShowDate, req.ShowWeather, req.WeatherLat, req.WeatherLon)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusCreated, device)
}

// PUT /api/devices/:id
func (h *DeviceHandler) UpdateDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name               string  `json:"name"`
		Host               string  `json:"host"`
		Width              int     `json:"width"`
		Height             int     `json:"height"`
		Orientation        string  `json:"orientation"`
		UseDeviceParameter bool    `json:"use_device_parameter"`
		EnableCollage      bool    `json:"enable_collage"` // Add this field
		ShowDate           bool    `json:"show_date"`
		ShowWeather        bool    `json:"show_weather"`
		WeatherLat         float64 `json:"weather_lat"`
		WeatherLon         float64 `json:"weather_lon"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	device, err := h.deviceService.UpdateDevice(uint(id), req.Name, req.Host, req.Width, req.Height, req.Orientation, req.UseDeviceParameter, req.EnableCollage, req.ShowDate, req.ShowWeather, req.WeatherLat, req.WeatherLon)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, device)
}

// DELETE /api/devices/:id
func (h *DeviceHandler) DeleteDevice(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.deviceService.DeleteDevice(uint(id)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	imagePath := req.URL
	var tempFile string // If we create a temp file, we must clean it up

	if req.ImageID != 0 {
		var img model.Image
		if err := h.db.First(&img, req.ImageID).Error; err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "image not found"})
		}

		if img.Source == model.SourceSynologyPhotos {
			// Download to temporary file
			data, err := h.synologyService.DownloadPhoto(int(img.SynologyPhotoID))
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to download synology photo: %v", err)})
			}

			// Save to temp file
			tmp, err := ioutil.TempFile("", "syno_push_*.jpg")
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file"})
			}
			defer os.Remove(tmp.Name()) // Clean up
			tempFile = tmp.Name()

			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write temp file"})
			}
			tmp.Close()
			imagePath = tempFile
		} else {
			imagePath = img.FilePath
		}
	}

	if imagePath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "image path or id required"})
	}

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "image file not found on server"})
	}

	// Push
	if err := h.deviceService.PushToDevice(uint(deviceID), imagePath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("push failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "pushed"})
}
