package handler

import (
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/labstack/echo/v4"
	xdraw "golang.org/x/image/draw"
	"gorm.io/gorm"
)

type GalleryHandler struct {
	db       *gorm.DB
	synology *service.SynologyService
	dataDir  string
}

func NewGalleryHandler(db *gorm.DB, synology *service.SynologyService, dataDir string) *GalleryHandler {
	return &GalleryHandler{
		db:       db,
		synology: synology,
		dataDir:  dataDir,
	}
}

// ListPhotos returns a paginated list of photos, optionally filtered by source
func (h *GalleryHandler) ListPhotos(c echo.Context) error {
	limit := 50
	offset := 0
	source := c.QueryParam("source")

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

	query := h.db.Model(&model.Image{})
	if source != "" {
		query = query.Where("source = ?", source)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to count photos"})
	}

	var items []model.Image
	if err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
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
		Source       string    `json:"source"`
	}

	var photos []PhotoResponse
	for _, item := range items {
		photos = append(photos, PhotoResponse{
			ID:           item.ID,
			ThumbnailURL: fmt.Sprintf("api/gallery/thumbnail/%d", item.ID),
			CreatedAt:    item.CreatedAt,
			Caption:      item.Caption,
			Width:        item.Width,
			Height:       item.Height,
			Orientation:  item.Orientation,
			Source:       item.Source,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"photos": photos,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetThumbnail serves the thumbnail for a photo.
// If it's a local/google photo, it serves/generates from disk.
// If it's a Synology photo, it proxies from Synology API.
func (h *GalleryHandler) GetThumbnail(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	var item model.Image
	if err := h.db.First(&item, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "photo not found"})
	}

	// Case 1: Synology (Proxy)
	if item.Source == model.SourceSynologyPhotos {
		// Synology thumbnail is fetched via service
		// We request 'small' (typically ~256px) or 'medium'
		// Synology sizes: small, medium, large, original
		thumbBytes, err := h.synology.GetPhoto(item.SynologyPhotoID, item.ThumbnailKey, "small")
		if err != nil {
			fmt.Printf("Failed to fetch synology thumbnail (ID=%d): %v\n", item.SynologyPhotoID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch synology thumbnail"})
		}
		c.Response().Header().Set("Content-Type", "image/jpeg")
		c.Response().Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
		_, err = c.Response().Write(thumbBytes)
		return err
	}

	// Case 2: Local File (Google/Local)
	thumbPath := filepath.Join(h.dataDir, "thumbnails", fmt.Sprintf("%d.jpg", item.ID))

	// Check cache
	if _, err := os.Stat(thumbPath); err == nil {
		c.Response().Header().Set("Cache-Control", "public, max-age=86400")
		return c.File(thumbPath)
	}

	// Generate from high-res file if missing
	if item.FilePath == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "source file missing"})
	}

	if err := h.generateThumbnail(item.FilePath, thumbPath); err != nil {
		fmt.Printf("Thumbnail generation failed for %d: %v\n", item.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate thumbnail"})
	}

	c.Response().Header().Set("Cache-Control", "public, max-age=86400")
	return c.File(thumbPath)
}

func (h *GalleryHandler) generateThumbnail(srcPath, destPath string) error {
	thumbsDir := filepath.Dir(destPath)
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		return err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	// Resize logic (fit 400x240)
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

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return jpeg.Encode(out, dst, &jpeg.Options{Quality: 80})
}

// DeletePhoto deletes a single photo
func (h *GalleryHandler) DeletePhoto(c echo.Context) error {
	id := c.Param("id")
	var item model.Image
	if err := h.db.First(&item, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "photo not found"})
	}

	// If local, delete file
	if item.Source == model.SourceGooglePhotos {
		if item.FilePath != "" {
			os.Remove(item.FilePath)
		}
		// Also delete thumbnail
		thumbPath := filepath.Join(h.dataDir, "thumbnails", fmt.Sprintf("%d.jpg", item.ID))
		os.Remove(thumbPath)
	}
	// For Synology, we just remove the DB reference, we don't delete from NAS.
	// For all (including google where we already deleted file), perform Unscoped delete from DB
	if err := h.db.Unscoped().Delete(&item).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete from db"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// DeletePhotos deletes all photos matching a source filter (or all if no filter)
// e.g. DELETE /api/gallery/photos?source=google
func (h *GalleryHandler) DeletePhotos(c echo.Context) error {
	source := c.QueryParam("source")

	var items []model.Image
	query := h.db.Model(&model.Image{})
	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&items).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to find photos"})
	}

	for _, item := range items {
		if item.Source == model.SourceGooglePhotos {
			if item.FilePath != "" {
				os.Remove(item.FilePath)
			}
			thumbPath := filepath.Join(h.dataDir, "thumbnails", fmt.Sprintf("%d.jpg", item.ID))
			os.Remove(thumbPath)
		}
	}

	// Delete from DB in a fresh transaction/query to avoid side effects
	delQuery := h.db
	if source != "" {
		delQuery = delQuery.Where("source = ?", source)
	}
	// Use Unscoped to ensure permanent delete
	if err := delQuery.Unscoped().Delete(&model.Image{}).Error; err != nil {
		fmt.Printf("DeletePhotos failed: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete from db"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "deleted",
		"count":   len(items),
		"message": fmt.Sprintf("Deleted %d photos", len(items)),
	})
}

// URL Proxy Handlers

type CreateURLSourceRequest struct {
	URL       string `json:"url"`
	DeviceIDs []uint `json:"device_ids"`
}

func (h *GalleryHandler) CreateURLSource(c echo.Context) error {
	req := new(CreateURLSourceRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.URL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "url is required"})
	}

	// Create URL Source Record
	src := model.URLSource{
		URL:       req.URL,
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(&src).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create url source"})
	}

	// Create Bindings
	if len(req.DeviceIDs) > 0 {
		for _, devID := range req.DeviceIDs {
			mapping := model.DeviceURLMapping{
				DeviceID:    devID,
				URLSourceID: src.ID,
			}
			if err := h.db.Create(&mapping).Error; err != nil {
				fmt.Printf("Failed to create binding for dev %d url %d: %v\n", devID, src.ID, err)
			}
		}
	}

	return c.JSON(http.StatusCreated, src)
}

func (h *GalleryHandler) ListURLSources(c echo.Context) error {
	var sources []model.URLSource
	if err := h.db.Find(&sources).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list url sources"})
	}

	// Fetch Mappings
	type URLSourceResponse struct {
		ID        uint      `json:"id"`
		URL       string    `json:"url"`
		CreatedAt time.Time `json:"created_at"`
		DeviceIDs []uint    `json:"device_ids"`
	}

	var resp []URLSourceResponse

	// Collecting Bindings
	// Opt: Pre-fetch all mappings
	mappings := []model.DeviceURLMapping{}
	h.db.Find(&mappings)
	bindingMap := make(map[uint][]uint)
	for _, m := range mappings {
		bindingMap[m.URLSourceID] = append(bindingMap[m.URLSourceID], m.DeviceID)
	}

	for _, s := range sources {
		resp = append(resp, URLSourceResponse{
			ID:        s.ID,
			URL:       s.URL,
			CreatedAt: s.CreatedAt,
			DeviceIDs: bindingMap[s.ID],
		})
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *GalleryHandler) DeleteURLSource(c echo.Context) error {
	id := c.Param("id")
	// Delete mappings
	h.db.Where("url_source_id = ?", id).Delete(&model.DeviceURLMapping{})
	// Delete source
	if err := h.db.Delete(&model.URLSource{}, id).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete url source"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *GalleryHandler) UpdateURLSource(c echo.Context) error {
	id := c.Param("id")
	req := new(CreateURLSourceRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.URL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "url is required"})
	}

	// Update Source
	var src model.URLSource
	if err := h.db.First(&src, id).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "url source not found"})
	}
	src.URL = req.URL
	if err := h.db.Save(&src).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update url source"})
	}

	// Re-create bindings
	// 1. Delete old
	h.db.Where("url_source_id = ?", id).Delete(&model.DeviceURLMapping{})

	// 2. Add new
	if len(req.DeviceIDs) > 0 {
		for _, devID := range req.DeviceIDs {
			mapping := model.DeviceURLMapping{
				DeviceID:    devID,
				URLSourceID: src.ID,
			}
			h.db.Create(&mapping)
		}
	}

	return c.JSON(http.StatusOK, src)
}
