package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	settings       *service.SettingsService
	telegram       *service.TelegramService
	google         *googlephotos.Client
	googleCalendar *googlephotos.Client
}

func NewHandler(s *service.SettingsService, t *service.TelegramService, g *googlephotos.Client, gc *googlephotos.Client) *Handler {
	return &Handler{settings: s, telegram: t, google: g, googleCalendar: gc}
}

// HealthCheck
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetSettings(c echo.Context) error {
	settings, err := h.settings.GetAll()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	if h.google.IsConnected() {
		settings["google_connected"] = "true"
	} else {
		settings["google_connected"] = "false"
	}

	if h.googleCalendar.IsConnected() {
		settings["google_calendar_connected"] = "true"
	} else {
		settings["google_calendar_connected"] = "false"
	}

	return c.JSON(http.StatusOK, settings)
}

type UpdateSettingsRequest struct {
	Settings map[string]string `json:"settings"`
}

func (h *Handler) UpdateSettings(c echo.Context) error {
	var req UpdateSettingsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	for k, v := range req.Settings {
		if err := h.settings.Set(k, v); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		// Dynamic Telegram Restart
		if k == "telegram_bot_token" {
			go h.telegram.Restart(v)
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}
