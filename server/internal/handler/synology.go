package handler

import (
	"net/http"
	"strings"

	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/labstack/echo/v4"
)

type SynologyHandler struct {
	synology *service.SynologyService
}

func NewSynologyHandler(s *service.SynologyService) *SynologyHandler {
	return &SynologyHandler{synology: s}
}

type TestConnectionRequest struct {
	OTPCode string `json:"otp_code"`
}

func (h *SynologyHandler) TestConnection(c echo.Context) error {
	var req TestConnectionRequest
	if err := c.Bind(&req); err != nil {
		// If bind fails, maybe no body?
		// Continue with empty OTP
	}

	if err := h.synology.TestConnection(req.OTPCode); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SynologyHandler) Logout(c echo.Context) error {
	if err := h.synology.Logout(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SynologyHandler) ListAlbums(c echo.Context) error {
	albums, err := h.synology.ListAlbums()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, albums)
}

func (h *SynologyHandler) Sync(c echo.Context) error {
	// Always clear and resync to ensure fresh references
	// Synology photos aren't stored locally, just references in DB
	if err := h.synology.ClearAndResync(); err != nil {
		// Check if it's an authentication error
		if strings.Contains(err.Error(), "authentication expired") {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Session expired. Please reconnect to Synology."})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "synced"})
}

func (h *SynologyHandler) Clear(c echo.Context) error {
	if err := h.synology.ClearPhotos(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *SynologyHandler) GetPhotoCount(c echo.Context) error {
	count, err := h.synology.GetPhotoCount()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"count": count})
}
