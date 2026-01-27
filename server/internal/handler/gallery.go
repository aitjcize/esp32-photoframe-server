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
	host := c.Request().Host
	for _, item := range items {
		photos = append(photos, PhotoResponse{
			ID:           item.ID,
			ThumbnailURL: fmt.Sprintf("http://%s/api/gallery/thumbnail/%d", host, item.ID),
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
	if item.Source == "synology" {
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

	// If local/google, delete file
	if item.Source == "google" || item.Source == "local" {
		if item.FilePath != "" {
			os.Remove(item.FilePath)
		}
		// Also delete thumbnail
		thumbPath := filepath.Join(h.dataDir, "thumbnails", fmt.Sprintf("%d.jpg", item.ID))
		os.Remove(thumbPath)
	}
	// For Synology, we just remove the DB reference, we don't delete from NAS.
	// For all (including local/google where we already deleted file), perform Unscoped delete from DB
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
		if item.Source == "google" || item.Source == "local" {
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
