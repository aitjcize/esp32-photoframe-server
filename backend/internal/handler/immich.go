package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
)

// ImmichHandler handles Immich-related API requests.
type ImmichHandler struct {
	immich *service.ImmichService
}

// NewImmichHandler creates a new ImmichHandler.
func NewImmichHandler(immich *service.ImmichService) *ImmichHandler {
	return &ImmichHandler{immich: immich}
}

// TestConnection tests the connection to an Immich server.
func (h *ImmichHandler) TestConnection(c echo.Context) error {
	var req struct {
		ServerURL string `json:"server_url"`
		APIKey    string `json:"api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.ServerURL == "" || req.APIKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "server_url and api_key are required"})
	}

	if err := h.immich.TestConnection(req.ServerURL, req.APIKey); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ListAlbums returns a list of albums from the configured Immich server.
func (h *ImmichHandler) ListAlbums(c echo.Context) error {
	var req struct {
		ServerURL string `json:"server_url"`
		APIKey    string `json:"api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.ServerURL == "" || req.APIKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "server_url and api_key are required"})
	}

	albums, err := h.immich.ListAlbums(req.ServerURL, req.APIKey)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, albums)
}
