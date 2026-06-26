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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ImmichHandler) ListAlbums(c echo.Context) error {
	albums, err := h.immich.ListAlbums()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if err := h.immich.SetSyncAlbums(req.AlbumIDs, req.Favorites, req.All, req.Memories); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ImmichHandler) Sync(c echo.Context) error {
	if err := h.immich.ClearAndResync(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "synced"})
}

func (h *ImmichHandler) Clear(c echo.Context) error {
	if err := h.immich.ClearPhotos(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *ImmichHandler) GetPhotoCount(c echo.Context) error {
	count, err := h.immich.GetPhotoCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"count": count})
}
