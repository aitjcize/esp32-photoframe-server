package handler

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"strconv"

	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/aitjcize/photoframe-server/server/pkg/googlephotos"
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
	// Determine Source (Route param overrides setting)
	forcedSource := c.Param("source")
	source, _ := h.settings.Get("source")
	if forcedSource != "" {
		source = forcedSource
	}
	if source == "" {
		return c.NoContent(http.StatusNotFound)
	}

	// 1. Fetch Random Photo (or pair if collage)
	collageMode, _ := h.settings.Get("collage_mode")

	var img image.Image
	var err error

	// Get target dimensions for the device
	orientation := h.getOrientation()
	targetW, targetH := 800, 480
	if orientation == "portrait" {
		targetW, targetH = 480, 800
	}

	if source == "telegram" {
		// Serve Telegram Photo (always single, no collage)
		imgPath := filepath.Join(h.dataDir, "photos", "telegram_last.jpg")
		f, fsErr := os.Open(imgPath)
		if fsErr != nil {
			// Fallback if no telegram photo yet
			img, err = h.fetchPlaceholder()
		} else {
			defer f.Close()
			img, _, err = image.Decode(f)
		}
	} else if collageMode == "true" {
		img, _, err = h.fetchSmartCollage(targetW, targetH, source)
	} else {
		img, _, err = h.fetchRandomPhoto(source)
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch photo: " + err.Error()})
	}

	// 1.5. Resize/Crop to Target Dimensions
	// This ensures the overlay is drawn on the FINAL visible area and not cropped later.
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	h.drawCover(dst, dst.Bounds(), img)
	img = dst

	// 2. Overlay
	imgWithOverlay, err := h.overlay.ApplyOverlay(img)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "overlay failed: " + err.Error()})
	}

	// 3. Tone Mapping + Thumbnail (CLI)
	processedBytes, thumbBytes, err := h.processor.ProcessImage(imgWithOverlay, nil)
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

	return c.Blob(http.StatusOK, "image/jpeg", data)
}

func (h *ImageHandler) ListGooglePhotos(c echo.Context) error {
	// Parse pagination parameters
	limit := 50 // default
	offset := 0

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get total count of Google Photos only
	var total int64
	if err := h.db.Model(&model.Image{}).Where("source = ?", "google").Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to count photos"})
	}

	// Get paginated Google Photos only
	var items []model.Image
	if err := h.db.Where("source = ?", "google").Order("created_at desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list photos"})
	}

	type PhotoResponse struct {
		ID           uint      `json:"id"`
		ThumbnailURL string    `json:"thumbnail_url"`
		CreatedAt    time.Time `json:"created_at"`
		Caption      string    `json:"caption"`
		Width        int       `json:"width"`
		Height       int       `json:"height"`
		Orientation  string    `json:"orientation"`
	}

	var photos []PhotoResponse
	host := c.Request().Host
	for _, item := range items {
		photos = append(photos, PhotoResponse{
			ID:           item.ID,
			ThumbnailURL: fmt.Sprintf("http://%s/api/google-photos/%d/thumbnail", host, item.ID),
			CreatedAt:    item.CreatedAt,
			Caption:      item.Caption,
			Width:        item.Width,
			Height:       item.Height,
			Orientation:  item.Orientation,
		})
	}

	// Return paginated response with metadata
	return c.JSON(http.StatusOK, map[string]interface{}{
		"photos": photos,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
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
func (h *ImageHandler) fetchSmartCollage(screenW, screenH int, sourceFilter string) (image.Image, uint, error) {
	// Decide if Device is Landscape or Portrait
	devicePortrait := screenH > screenW

	// Fetch first image
	img1, id1, err := h.fetchRandomPhoto(sourceFilter)
	if err != nil {
		return nil, 0, err
	}

	bounds := img1.Bounds()
	w, h_img := bounds.Dx(), bounds.Dy()
	isPhotoPortrait := h_img > w

	// Case 1: Match
	if isPhotoPortrait == devicePortrait {
		return img1, id1, nil
	}

	// Case 2: Mismatch
	// Device Portrait, Photo Landscape -> Vertical Stack
	if devicePortrait && !isPhotoPortrait {
		// Try fetch second landscape
		// 1. Try DB first
		img2, id2, err := h.fetchRandomPhotoWithType("landscape", sourceFilter)
		if err == nil && id2 != id1 {
			return h.createVerticalCollage(img1, img2), 0, nil
		}
		// 2. Fallback: Try random loop
		for i := 0; i < 5; i++ {
			cand, candID, err := h.fetchRandomPhoto(sourceFilter)
			if err == nil && candID != id1 {
				b := cand.Bounds()
				if b.Dx() > b.Dy() { // Is Landscape
					// fmt.Printf("SmartCollage: Found match via random!\n")
					return h.createVerticalCollage(img1, cand), 0, nil
				}
			}
		}
		// fmt.Printf("SmartCollage: Failed to find partner, returning single.\n")
	}

	// Device Landscape, Photo Portrait -> Horizontal Side-by-Side
	if !devicePortrait && isPhotoPortrait {
		// Try fetch second portrait
		img2, id2, err := h.fetchRandomPhotoWithType("portrait", sourceFilter)
		if err == nil && id2 != id1 {
			return h.createHorizontalCollage(img1, img2), 0, nil
		}
		// 2. Fallback
		for i := 0; i < 5; i++ {
			cand, candID, err := h.fetchRandomPhoto(sourceFilter)
			if err == nil && candID != id1 {
				b := cand.Bounds()
				if b.Dy() > b.Dx() { // Is Portrait
					// fmt.Printf("SmartCollage: Found match via random!\n")
					return h.createHorizontalCollage(img1, cand), 0, nil
				}
			}
		}
		// fmt.Printf("SmartCollage: Failed to find partner, returning single.\n")
	}

	return img1, id1, nil
}

func (h *ImageHandler) fetchRandomPhotoWithType(targetType string, sourceFilter string) (image.Image, uint, error) {
	var item model.Image
	query := h.db.Order("RANDOM()").Where("orientation = ?", targetType)

	if sourceFilter == "google_photos" || sourceFilter == "" {
		query = query.Where("source = ? OR source = ?", "google", "")
	} else if sourceFilter == "synology" {
		query = query.Where("source = ?", "synology")
	} else if sourceFilter == "telegram" {
		query = query.Where("source = ?", "telegram")
	}

	if err := query.First(&item).Error; err != nil {
		return nil, 0, err
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

func (h *ImageHandler) createVerticalCollage(img1, img2 image.Image) image.Image {
	// Target Dimension: 480x800 (Portrait)
	// Each slot: 480x400
	width := 480
	height := 800
	slotHeight := 400

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw Top
	h.drawCover(dst, image.Rect(0, 0, width, slotHeight), img1)

	// Draw Bottom
	h.drawCover(dst, image.Rect(0, slotHeight, width, height), img2)

	return dst
}

func (h *ImageHandler) createHorizontalCollage(img1, img2 image.Image) image.Image {
	// Target Dimension: 800x480 (Landscape)
	// Each slot: 400x480
	width := 800
	height := 480
	slotWidth := 400

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Draw Left
	h.drawCover(dst, image.Rect(0, 0, slotWidth, height), img1)

	// Draw Right
	h.drawCover(dst, image.Rect(slotWidth, 0, width, height), img2)

	return dst
}

func (h *ImageHandler) drawCover(dst draw.Image, r image.Rectangle, src image.Image) {
	// Calculate scaling to cover 'r'
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	dstW, dstH := r.Dx(), r.Dy()

	// Calculate crop rect
	var srcCrop image.Rectangle
	if float64(srcW)/float64(srcH) > float64(dstW)/float64(dstH) {
		// Source is wider than target: Crop width
		matchW := int(float64(srcH) * float64(dstW) / float64(dstH))
		midX := srcW / 2
		srcCrop = image.Rect(midX-matchW/2, 0, midX+matchW/2, srcH)
	} else {
		// Source is taller: Crop height
		matchH := int(float64(srcW) * float64(dstH) / float64(dstW))
		midY := srcH / 2
		srcCrop = image.Rect(0, midY-matchH/2, srcW, midY+matchH/2)
	}

	// Implementation of simple Nearest Neighbor scaler:
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			// Percentages in Destination
			pX := float64(x) / float64(dstW)
			pY := float64(y) / float64(dstH)

			// Source coords in original image via Crop
			sX := srcCrop.Min.X + int(pX*float64(srcCrop.Dx()))
			sY := srcCrop.Min.Y + int(pY*float64(srcCrop.Dy()))

			// Bounds check safety
			if sX < 0 {
				sX = 0
			}
			if sY < 0 {
				sY = 0
			}
			if sX >= srcW {
				sX = srcW - 1
			}
			if sY >= srcH {
				sY = srcH - 1
			}

			dst.Set(r.Min.X+x, r.Min.Y+y, src.At(srcBounds.Min.X+sX, srcBounds.Min.Y+sY))
		}
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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

func (h *ImageHandler) fetchRandomPhoto(sourceFilter string) (image.Image, uint, error) {
	// Source logic: if "google_photos" (default), we include source="google" OR source="" (legacy)
	// If "synology", source="synology"
	// If "telegram", source="telegram"

	// Note: settings uses "google_photos" but DB uses "google"? Or "local"?
	// Legacy: empty source is usually local or google.
	// We need to check data model.

	var item model.Image
	query := h.db.Order("RANDOM()")

	if sourceFilter == "google_photos" {
		query = query.Where("source = ? OR source = ?", "google", "")
	} else if sourceFilter == "synology" {
		query = query.Where("source = ?", "synology")
	} else if sourceFilter == "telegram" {
		query = query.Where("source = ?", "telegram")
	}
	// If empty filter passed (unlikely if ServeImage handles it), fallback?

	result := query.First(&item)
	if result.Error != nil {
		img, err := h.fetchPlaceholder()
		return img, 0, err
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
