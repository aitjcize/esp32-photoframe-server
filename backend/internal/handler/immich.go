package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
)

type ImmichHandler struct {
	immich *service.ImmichService
}

func NewImmichHandler(s *service.ImmichService) *ImmichHandler {
	return &ImmichHandler{immich: s}
}

func (h *ImmichHandler) TestConnection(c echo.Context) error {
	if err := h.immich.TestConnection(); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ImmichHandler) ListAlbums(c echo.Context) error {
	albums, err := h.immich.ListAlbums()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, albums)
}

// SetSyncAlbums defines which Immich albums (real + virtual modes) to sync.
// POST /api/immich/sync-albums
func (h *ImmichHandler) SetSyncAlbums(c echo.Context) error {
	var req struct {
		AlbumIDs  []string `json:"album_ids"`
		Favorites bool     `json:"favorites"`
		All       bool     `json:"all"`
		Memories  bool     `json:"memories"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if err := h.immich.SetSyncAlbums(req.AlbumIDs, req.Favorites, req.All, req.Memories); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// CacheStatus returns the current state of the Immich image cache.
// GET /api/immich/cache/status
func (h *ImmichHandler) CacheStatus(c echo.Context) error {
	cache := h.immich.GetCacheService()
	if cache == nil {
		return c.JSON(http.StatusOK, service.CacheStatus{Enabled: false})
	}
	return c.JSON(http.StatusOK, cache.Status())
}

// CachePopulate triggers an immediate background cache population cycle.
// POST /api/immich/cache/populate
func (h *ImmichHandler) CachePopulate(c echo.Context) error {
	cache := h.immich.GetCacheService()
	if cache == nil {
		return respondError(c, http.StatusBadRequest, "cache service not available")
	}
	if !cache.Enabled() {
		return respondError(c, http.StatusBadRequest, "immich cache is not enabled")
	}
	cache.PopulateNow()
	return c.JSON(http.StatusOK, map[string]string{"status": "population started"})
}

// CacheClear deletes all cached images from disk and the database.
// POST /api/immich/cache/clear
func (h *ImmichHandler) CacheClear(c echo.Context) error {
	cache := h.immich.GetCacheService()
	if cache == nil {
		return respondError(c, http.StatusBadRequest, "cache service not available")
	}
	if err := cache.ClearCache(); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cache cleared"})
}

// Sync / SyncStatus / Clear / Count are served by the generic PhotoSyncHandler
// (see photosync_handler.go) — identical across photo sources.
