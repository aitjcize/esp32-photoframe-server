package handler

import (
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/labstack/echo/v4"
)

type CalendarHandler struct {
	google   *googlephotos.Client
	calendar *gcalendar.Client
}

func NewCalendarHandler(google *googlephotos.Client, calendar *gcalendar.Client) *CalendarHandler {
	return &CalendarHandler{
		google:   google,
		calendar: calendar,
	}
}

// ListCalendars returns the user's Google Calendar list.
// Returns 403 with {"error": "calendar_not_authorized"} if the token lacks calendar scope.
func (h *CalendarHandler) ListCalendars(c echo.Context) error {
	httpClient, err := h.google.GetClient()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "google calendar not connected"})
	}

	calendars, err := h.calendar.GetCalendarList(httpClient)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if calendars == nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "calendar_not_authorized"})
	}

	return c.JSON(http.StatusOK, calendars)
}
