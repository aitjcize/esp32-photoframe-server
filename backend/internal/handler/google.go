package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/labstack/echo/v4"
)

// GoogleHandler serves the Google OAuth endpoints: the Calendar login and
// logout, and the callback that Photos and Calendar share. Google Photos
// logins start per connection under /api/connections/:id/google/login; the
// callback routes their codes back by the state nonce the manager issued.
type GoogleHandler struct {
	calendarClient *googlephotos.Client
	conns          *service.ConnectionManager
}

func NewGoogleHandler(calendarClient *googlephotos.Client, conns *service.ConnectionManager) *GoogleHandler {
	return &GoogleHandler{calendarClient: calendarClient, conns: conns}
}

// CalendarLogin initiates the OAuth flow for Google Calendar.
func (h *GoogleHandler) CalendarLogin(c echo.Context) error {
	redirectURL := buildRedirectURL(c)
	h.calendarClient.SetRedirectURL(redirectURL)
	url := h.calendarClient.GetAuthURL("calendar")
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

// Callback handles the shared OAuth callback. The state parameter says whose
// code this is: the literal "calendar", or the nonce of a pending Google
// Photos connection login.
func (h *GoogleHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" {
		return respondError(c, http.StatusBadRequest, "code is required")
	}

	if state == "calendar" {
		if err := h.calendarClient.Exchange(code); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		return c.Redirect(http.StatusTemporaryRedirect, "/?tab=datasources&source=google")
	}

	id, err := h.conns.CompleteGoogleAuth(state, code)
	if err != nil {
		if errors.Is(err, service.ErrUnknownOAuthState) {
			return respondError(c, http.StatusBadRequest, "This sign-in link has expired. Open the dashboard and click Connect again.")
		}
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	// Back to the Google tab, on the card that was just connected.
	return c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?tab=datasources&source=google&connection=%d", id))
}

func (h *GoogleHandler) CalendarLogout(c echo.Context) error {
	if err := h.calendarClient.Logout(); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func buildRedirectURL(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request().Host + "/api/auth/google/callback"
}
