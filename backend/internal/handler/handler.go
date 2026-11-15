package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/version"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	settings *service.SettingsService
	telegram *service.TelegramService
}

func NewHandler(s *service.SettingsService, t *service.TelegramService) *Handler {
	return &Handler{settings: s, telegram: t}
}

// HealthCheck reports liveness plus the build-time server version (issue #6:
// lets users on the "latest" image tag see which version they're running).
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "ok",
		"version": version.Version,
	})
}

func (h *Handler) GetSettings(c echo.Context) error {
	settings, err := h.settings.GetAll()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, settings)
}

type UpdateSettingsRequest struct {
	Settings map[string]string `json:"settings"`
}

func (h *Handler) UpdateSettings(c echo.Context) error {
	var req UpdateSettingsRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "Invalid request")
	}

	for k, v := range req.Settings {
		if err := h.settings.Set(k, v); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}

		// Dynamic Telegram Restart
		if k == "telegram_bot_token" {
			go h.telegram.Restart(v)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}
