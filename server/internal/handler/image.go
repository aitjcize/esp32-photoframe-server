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

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/aitjcize/photoframe-server/server/pkg/googlephotos"
	"github.com/aitjcize/photoframe-server/server/pkg/imageops"
	"github.com/aitjcize/photoframe-server/server/pkg/photoframe"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type ImageHandler struct {
	settings  *service.SettingsService
	overlay   *service.OverlayService
	processor *service.ProcessorService
	google    *googlephotos.Client
	synology  *service.SynologyService
	db        *gorm.DB
	dataDir   string
}

func NewImageHandler(
	s *service.SettingsService,
	o *service.OverlayService,
	p *service.ProcessorService,
	g *googlephotos.Client,
	synology *service.SynologyService,
	db *gorm.DB,
	dataDir string,
) *ImageHandler {
	return &ImageHandler{
		settings:  s,
		overlay:   o,
		processor: p,
		google:    g,
		synology:  synology,
		db:        db,
		dataDir:   dataDir,
	}
}

func (h *ImageHandler) ServeImage(c echo.Context) error {
	// Get source from route parameter
	source := c.Param("source")

	// 1. Identify Device and Determine Settings
	// Try to find device by Hostname (X-Hostname header) first, then IP
	var device model.Device
	var result *gorm.DB

	hostname := c.Request().Header.Get("X-Hostname")
	if hostname != "" {
		// Try matching Host or Name? Host in DB is often hostname.
		result = h.db.Where("host = ?", hostname).First(&device)
	}

	// If not found by hostname, try by IP
	deviceFound := result != nil && result.Error == nil
	if !deviceFound {
		clientIP := c.RealIP()
		result = h.db.Where("host = ?", clientIP).First(&device)
		deviceFound = result.Error == nil
	}

	// Native resolution of the device panel
	nativeW, nativeH := 800, 480
	// Logical resolution for image generation (respects orientation)
	logicalW, logicalH := 800, 480

	enableCollage := false
	showDate := false
	showWeather := false
	var lat, lon float64

	if deviceFound {
		nativeW = device.Width
		nativeH = device.Height
		logicalW, logicalH = nativeW, nativeH

		enableCollage = device.EnableCollage
		showDate = device.ShowDate
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
	if oStr := c.Request().Header.Get("X-Display-Orientation"); oStr != "" {
		if oStr == "portrait" && logicalW > logicalH {
			logicalW, logicalH = logicalH, logicalW
		} else if oStr == "landscape" && logicalW < logicalH {
			logicalW, logicalH = logicalH, logicalW
		}
		// Persist orientation update to database if it changed
		if deviceFound && device.Orientation != oStr {
			device.Orientation = oStr
			h.db.Model(&device).Update("orientation", oStr)
		}
	} else if deviceFound && device.Orientation != "" {
		// Use device orientation preference if no header provided
		if device.Orientation == "portrait" && logicalW > logicalH {
			logicalW, logicalH = logicalH, logicalW
		} else if device.Orientation == "landscape" && logicalW < logicalH {
			logicalW, logicalH = logicalH, logicalW
		}
	}

	var img image.Image
	var err error

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

	if source == "telegram" {
		// Serve Telegram Photo (always single, no collage)
		imgPath := filepath.Join(h.dataDir, "photos", "telegram_last.jpg")
		f, fsErr := os.Open(imgPath)
		if fsErr != nil {
			img, err = h.fetchPlaceholder()
		} else {
			defer f.Close()
			img, _, err = image.Decode(f)
		}
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

	// 1.6. Record History
	if deviceFound && len(servedImageIDs) > 0 {
		go func(devID uint, imgIDs []uint) {
			for _, imgID := range imgIDs {
				if imgID == 0 {
					continue
				}
				h.db.Create(&model.DeviceHistory{
					DeviceID: devID,
					ImageID:  imgID,
					ServedAt: time.Now(),
				})
			}
			// Prune old history
			// Keep last 100 entries for this device
			// (Keep more in DB than we filter to have a buffer)
			var count int64
			h.db.Model(&model.DeviceHistory{}).Where("device_id = ?", devID).Count(&count)
			if count > 100 {
				// Delete oldest
				// SQLite modification with LIMIT is compile-option dependent, subquery is safer
				h.db.Where("device_id = ? AND id NOT IN (?)", devID,
					h.db.Model(&model.DeviceHistory{}).Select("id").
						Where("device_id = ?", devID).
						Order("served_at desc").
						Limit(100),
				).Delete(&model.DeviceHistory{})
			}
		}(device.ID, servedImageIDs)
	}

	// 1.5. Resize/Crop to Target Dimensions
	dst := image.NewRGBA(image.Rect(0, 0, logicalW, logicalH))
	imageops.DrawCover(dst, dst.Bounds(), img)
	img = dst

	// 2. Overlay
	overlayOpts := service.OverlayOptions{
		ShowDate:    showDate,
		ShowWeather: showWeather,
		WeatherLat:  lat,
		WeatherLon:  lon,
	}

	imgWithOverlay, err := h.overlay.ApplyOverlay(img, overlayOpts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "overlay failed: " + err.Error()})
	}

	// 3. Tone Mapping + Thumbnail (CLI)
	// Pass NATIVE dimensions to CLI.
	// The CLI will detect Source (logicalW/H) vs Target (nativeW/H) orientation mismatch and rotate if needed.
	procOptions := map[string]string{
		"dimension": fmt.Sprintf("%dx%d", nativeW, nativeH),
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

	// Set Content-Length header
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", len(processedBytes)))

	return c.Blob(http.StatusOK, "image/png", processedBytes)
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

// Fetch smart photo (Single or Collage)
func (h *ImageHandler) fetchSmartCollage(screenW, screenH int, sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, []uint, error) {
	// Decide if Device is Landscape or Portrait
	devicePortrait := screenH > screenW

	// Fetch first image generic (could be portrait or landscape)
	img1, id1, err := h.fetchRandomPhoto(sourceFilter, excludeIDs, deviceID)
	if err != nil {
		return nil, nil, err
	}

	var servedIDs []uint
	servedIDs = append(servedIDs, id1)

	// Add id1 to excludes for the second image
	excludeIDs2 := append([]uint(nil), excludeIDs...)
	excludeIDs2 = append(excludeIDs2, id1)

	bounds := img1.Bounds()
	w, h_img := bounds.Dx(), bounds.Dy()
	isPhotoPortrait := h_img > w

	// Case 1: Match
	if isPhotoPortrait == devicePortrait {
		return img1, servedIDs, nil
	}

	// Case 2: Mismatch
	// Device Portrait, Photo Landscape -> Vertical Stack
	if devicePortrait && !isPhotoPortrait {
		// Try fetch second landscape
		// 1. Try DB first
		img2, id2, err := h.fetchRandomPhotoWithType("landscape", sourceFilter, excludeIDs2, deviceID)
		if err == nil && id2 != id1 {
			servedIDs = append(servedIDs, id2)
			return h.createVerticalCollage(img1, img2, screenW, screenH), servedIDs, nil
		}
		// 2. Fallback: Try random loop
		for i := 0; i < 5; i++ {
			cand, candID, err := h.fetchRandomPhoto(sourceFilter, excludeIDs2, deviceID)
			if err == nil && candID != id1 {
				b := cand.Bounds()
				if b.Dx() > b.Dy() { // Is Landscape
					// fmt.Printf("SmartCollage: Found match via random!\n")
					servedIDs = append(servedIDs, candID)
					return h.createVerticalCollage(img1, cand, screenW, screenH), servedIDs, nil
				}
			}
		}
		// Fallback: Use same photo twice
		servedIDs = append(servedIDs, id1)
		return h.createVerticalCollage(img1, img1, screenW, screenH), servedIDs, nil
	}

	// Device Landscape, Photo Portrait -> Horizontal Side-by-Side
	if !devicePortrait && isPhotoPortrait {
		// Try fetch second portrait
		img2, id2, err := h.fetchRandomPhotoWithType("portrait", sourceFilter, excludeIDs2, deviceID)
		if err == nil && id2 != id1 {
			servedIDs = append(servedIDs, id2)
			return h.createHorizontalCollage(img1, img2, screenW, screenH), servedIDs, nil
		}
		// 2. Fallback
		for i := 0; i < 5; i++ {
			cand, candID, err := h.fetchRandomPhoto(sourceFilter, excludeIDs2, deviceID)
			if err == nil && candID != id1 {
				b := cand.Bounds()
				if b.Dy() > b.Dx() { // Is Portrait
					// fmt.Printf("SmartCollage: Found match via random!\n")
					servedIDs = append(servedIDs, candID)
					return h.createHorizontalCollage(img1, cand, screenW, screenH), servedIDs, nil
				}
			}
		}
		// Fallback: Use same photo twice
		servedIDs = append(servedIDs, id1)
		return h.createHorizontalCollage(img1, img1, screenW, screenH), servedIDs, nil
	}

	return img1, servedIDs, nil
}

func (h *ImageHandler) fetchRandomPhotoWithType(targetType string, sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, uint, error) {
	var item model.Image
	// Allow 'auto' orientation (e.g. for dynamic URLs) to match any target type
	query := h.db.Order("RANDOM()").Where("orientation IN ?", []string{targetType, "auto"})

	if len(excludeIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeIDs)
	}

	if sourceFilter == "google_photos" {
		query = query.Where("source = ?", "google")
	} else if sourceFilter == "synology" {
		query = query.Where("source = ?", "synology")
	} else if sourceFilter == "telegram" {
		query = query.Where("source = ?", "telegram")
	} else if sourceFilter == "url_proxy" {
		// For URL Proxy, we fetch from url_sources table
		var urlSource model.URLSource
		// Logic: Get all valid URL sources for this device
		// Valid = Global (not in mapping) OR Bound to this device

		// Subquery for valid IDs
		subQuery := h.db.Table("url_sources").Select("url_sources.*")
		if deviceID != nil {
			subQuery = subQuery.Joins("LEFT JOIN device_url_mappings ON url_sources.id = device_url_mappings.url_source_id").
				Where("device_url_mappings.device_id = ? OR device_url_mappings.device_id IS NULL", *deviceID)
		} else {
			// If no device ID (unknown device), strictly show only globals?
			// Or just show globals.
			subQuery = subQuery.Joins("LEFT JOIN device_url_mappings ON url_sources.id = device_url_mappings.url_source_id").
				Where("device_url_mappings.device_id IS NULL")
		}

		// Pick one random
		if err := subQuery.Order("RANDOM()").Take(&urlSource).Error; err != nil {
			return nil, 0, err
		}

		if urlSource.URL == "" {
			return nil, 0, fmt.Errorf("fetched empty URL from source ID %d", urlSource.ID)
		}
		// Use ID 0 for URL proxy items as they don't map to 'images' table IDs
		// BUT we need an ID for history tracking?
		// If we don't track history for URLs (since they are proxies), we can pass 0.
		// However, if we pass 0, exclude list logic won't work.
		// User said "don't need to consider randomization".
		// So maybe we don't track history for URL proxy?
		// Let's return local ID of url_source as ID, but effectively it's different namespace.
		// This might collide with image IDs in history.
		// For now, let's just fetch it.
		return h.fetchURLPhoto(urlSource.URL)
	} else {
		return nil, 0, fmt.Errorf("invalid source filter: %s", sourceFilter)
	}

	if err := query.First(&item).Error; err != nil {
		// Fallback: If no image found (likely due to exclusion), try again WITHOUT exclusion
		if len(excludeIDs) > 0 {
			queryRetry := h.db.Order("RANDOM()").Where("orientation = ?", targetType)

			if sourceFilter == "google_photos" {
				queryRetry = queryRetry.Where("source = ?", "google")
			} else if sourceFilter == "synology" {
				queryRetry = queryRetry.Where("source = ?", "synology")
			} else if sourceFilter == "telegram" {
				queryRetry = queryRetry.Where("source = ?", "telegram")
			} else if sourceFilter == "url_proxy" {
				queryRetry = queryRetry.Where("source = ?", "url_proxy")
				if deviceID != nil {
					queryRetry = queryRetry.Where("id IN (SELECT image_id FROM device_image_mappings WHERE device_id = ?) OR id NOT IN (SELECT image_id FROM device_image_mappings)", *deviceID)
				} else {
					queryRetry = queryRetry.Where("id NOT IN (SELECT image_id FROM device_image_mappings)")
				}
			}

			if errRetry := queryRetry.First(&item).Error; errRetry != nil {
				return nil, 0, errRetry
			}
		} else {
			return nil, 0, err
		}
	}

	resolvedPath := h.resolvePath(item.FilePath)
	f, err := os.Open(resolvedPath)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
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

func (h *ImageHandler) fetchRandomPhoto(sourceFilter string, excludeIDs []uint, deviceID *uint) (image.Image, uint, error) {
	// Source logic: if "google_photos" (default), we include source="google" OR source="" (legacy)
	// If "synology", source="synology"
	// If "telegram", source="telegram"

	// Note: settings uses "google_photos" but DB uses "google"? Or "local"?
	// Legacy: empty source is usually local or google.
	// We need to check data model.

	var item model.Image
	query := h.db.Order("RANDOM()")

	if len(excludeIDs) > 0 {
		query = query.Where("id NOT IN ?", excludeIDs)
	}

	if sourceFilter == "google_photos" {
		query = query.Where("source = ?", "google")
	} else if sourceFilter == "synology" {
		query = query.Where("source = ?", "synology")
	} else if sourceFilter == "telegram" {
		query = query.Where("source = ?", "telegram")
	} else if sourceFilter == "url_proxy" {
		// For URL Proxy, we fetch from url_sources table
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
		return h.fetchURLPhoto(urlSource.URL)
	} else {
		return nil, 0, fmt.Errorf("invalid source filter: %s", sourceFilter)
	}

	result := query.First(&item)
	if result.Error != nil {
		// Fallback: If no image found (likely due to exclusion), try again WITHOUT exclusion
		if len(excludeIDs) > 0 {
			queryRetry := h.db.Order("RANDOM()")

			if sourceFilter == "google_photos" {
				queryRetry = queryRetry.Where("source = ?", "google")
			} else if sourceFilter == "synology" {
				queryRetry = queryRetry.Where("source = ?", "synology")
			} else if sourceFilter == "telegram" {
				queryRetry = queryRetry.Where("source = ?", "telegram")
			} else if sourceFilter == "url_proxy" {
				// For URL Proxy, we fetch from url_sources table
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
				return h.fetchURLPhoto(urlSource.URL)
			}

			if errRetry := queryRetry.First(&item).Error; errRetry != nil {
				// Still failed, return placeholder
				img, err := h.fetchPlaceholder()
				return img, 0, err
			}
		} else {
			img, err := h.fetchPlaceholder()
			return img, 0, err
		}
	}

	if item.Source == "synology" {
		img, _, err := h.fetchSynologyPhoto(item)
		if err != nil {
			fmt.Printf("Warning: Failed to fetch Synology photo: %v\n", err)
			img, err := h.fetchPlaceholder()
			return img, 0, err
		}
		return img, item.ID, nil
	}
	resolvedPath := h.resolvePath(item.FilePath)
	f, err := os.Open(resolvedPath)
	if err != nil {
		// Do NOT delete the record just because file is missing locally
		// h.db.Delete(&item)
		fmt.Printf("Warning: Failed to open image: %s (resolved: %s): %v\n", item.FilePath, resolvedPath, err)
		img, err := h.fetchPlaceholder()
		return img, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		img, err := h.fetchPlaceholder()
		return img, 0, err
	}
	return img, item.ID, nil
}

func (h *ImageHandler) fetchPlaceholder() (image.Image, error) {
	resp, err := http.Get("https://picsum.photos/800/480")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
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
