package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/labstack/echo/v4"
)

// GoogleHandler serves the OAuth callback that every Google connection
// (Photos and Calendar accounts) shares. Logins start per connection under
// /api/connections/:id/google/login; the callback routes the code back by the
// state nonce the manager issued.
type GoogleHandler struct {
	conns *service.ConnectionManager
}

func NewGoogleHandler(conns *service.ConnectionManager) *GoogleHandler {
	return &GoogleHandler{conns: conns}
}

// Callback completes the sign-in the state names and sends the browser back
// to the dashboard: to the photo source for a Google Photos account, to the
// Calendar tab for a Google Calendar account.
func (h *GoogleHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" {
		return respondError(c, http.StatusBadRequest, "code is required")
	}

	id, err := h.conns.CompleteGoogleAuth(state, code)
	if err != nil {
		if errors.Is(err, service.ErrUnknownOAuthState) {
			return respondError(c, http.StatusBadRequest, "This sign-in link has expired. Open the dashboard and click Connect again.")
		}
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	conn, err := h.conns.Store().Get(id)
	if err == nil && conn.Source == model.SourceGoogleCalendar {
		return c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?tab=calendar&connection=%d", id))
	}
	return c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?source=google&connection=%d", id))
}

func buildRedirectURL(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request().Host + "/api/auth/google/callback"
}
