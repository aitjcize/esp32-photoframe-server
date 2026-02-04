package handler

import (
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/aitjcize/photoframe-server/server/pkg/googlephotos"
	"github.com/labstack/echo/v4"
	xdraw "golang.org/x/image/draw"
	"gorm.io/gorm"
)

type GoogleHandler struct {
	client  *googlephotos.Client
	picker  *service.PickerService
	db      *gorm.DB
	dataDir string
}

func NewGoogleHandler(client *googlephotos.Client, picker *service.PickerService, db *gorm.DB, dataDir string) *GoogleHandler {
	return &GoogleHandler{
		client:  client,
		picker:  picker,
		db:      db,
		dataDir: dataDir,
	}
}

func (h *GoogleHandler) Login(c echo.Context) error {
	// Construct redirect URL from request
	scheme := "http"
	if c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request().Host
	redirectURL := scheme + "://" + host + "/api/auth/google/callback"

	h.client.SetRedirectURL(redirectURL)
	url := h.client.GetAuthURL()
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

func (h *GoogleHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	if code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "code is required"})
	}

	if err := h.client.Exchange(code); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Redirect back to frontend
	return c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?tab=datasources&source=%s", model.SourceGooglePhotos))
}

func (h *GoogleHandler) Logout(c echo.Context) error {
	if err := h.client.Logout(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ListAlbums is no longer supported for Google Photos

func (h *GoogleHandler) CreatePickerSession(c echo.Context) error {
	id, uri, err := h.picker.CreateSession()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{
		"id":        id,
		"pickerUri": uri,
	})
}

func (h *GoogleHandler) PollPickerSession(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "session id required"})
	}

	complete, err := h.picker.PollSession(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if complete {
		// If complete, trigger download in background? Or do it synchronously?
		// Synchronously might time out.
		// Ideally: return "complete: true", frontend shows "Processing..."
		// Then frontend calls /api/google/picker/process/{id}
		return c.JSON(http.StatusOK, map[string]bool{"complete": true})
	}

	return c.JSON(http.StatusOK, map[string]bool{"complete": false})
}

func (h *GoogleHandler) ProcessPickerSession(c echo.Context) error {
	id := c.Param("id")

	// Check if already processing? For now blindly start.
	go func() {
		_, err := h.picker.ProcessSessionItems(id)
		if err != nil {
			// Error is recorded in progress state
			// fmt.Printf("ProcessSessionItems background error: %v\n", err)
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "processing"})
}

func (h *GoogleHandler) PollPickerProgress(c echo.Context) error {
	id := c.Param("id")
	progress := h.picker.GetProgress(id)
	if progress == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "session not found"})
	}
	return c.JSON(http.StatusOK, progress)
}

func (h *GoogleHandler) DeleteAllGooglePhotos(c echo.Context) error {
	var items []model.Image
	// Only fetch Google Photos
	if err := h.db.Where("source = ?", model.SourceGooglePhotos).Find(&items).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch photos"})
	}

	// Delete local files
	for _, item := range items {
		if item.FilePath != "" {
			os.Remove(item.FilePath)
		}
	}

	// Delete from DB
	if err := h.db.Where("source = ?", model.SourceGooglePhotos).Delete(&model.Image{}).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete photos from db"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "deleted",
		"message": fmt.Sprintf("Deleted %d Google photos", len(items)),
	})
}

func (h *GoogleHandler) DeleteGooglePhoto(c echo.Context) error {
	id := c.Param("id")
	var item model.Image
	if err := h.db.Where("source = ?", model.SourceGooglePhotos).First(&item, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "photo not found"})
	}

	// Delete file
	if item.FilePath != "" {
		os.Remove(item.FilePath)
	}

	// Delete from DB
	if err := h.db.Delete(&item).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete photo from db"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *GoogleHandler) GetGooglePhotoThumbnail(c echo.Context) error {
	id := c.Param("id")
	var item model.Image
	if err := h.db.Where("source = ?", model.SourceGooglePhotos).First(&item, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "photo not found"})
	}

	// 1. Check Cache
	thumbPath := filepath.Join(h.dataDir, "thumbnails", fmt.Sprintf("%s.jpg", id))
	if _, err := os.Stat(thumbPath); err == nil {
		return c.File(thumbPath)
	}

	// 2. Generate
	thumbsDir := filepath.Join(h.dataDir, "thumbnails")
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create thumb dir"})
	}

	f, err := os.Open(item.FilePath)
	if err != nil {
		fmt.Printf("Failed to open image for thumbnail: path=%s, error=%v\n", item.FilePath, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open image"})
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to decode image: " + err.Error()})
	}

	// Calculate 400x240 fit
	bounds := img.Bounds()
	ratio := float64(bounds.Dx()) / float64(bounds.Dy())
	targetH := 240
	targetW := int(float64(targetH) * ratio)
	if targetW > 400 {
		targetW = 400
		targetH = int(float64(targetW) / ratio)
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	// Save
	out, err := os.Create(thumbPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save thumb"})
	}
	defer out.Close()
	jpeg.Encode(out, dst, &jpeg.Options{Quality: 80})

	// 3. Serve
	return c.File(thumbPath)
}

func (h *GoogleHandler) ListGooglePhotos(c echo.Context) error {
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
	if err := h.db.Model(&model.Image{}).Where("source = ?", model.SourceGooglePhotos).Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to count photos"})
	}

	// Get paginated Google Photos only
	var items []model.Image
	if err := h.db.Where("source = ?", model.SourceGooglePhotos).Order("created_at desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
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
	for _, item := range items {
		photos = append(photos, PhotoResponse{
			ID:           item.ID,
			ThumbnailURL: fmt.Sprintf("api/google-photos/%d/thumbnail", item.ID),
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
