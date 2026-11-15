package service

import (
	"errors"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

// ErrCalendarNotAuthorized is returned when the account's token has no
// calendar scope (an account signed in before the scope was asked for).
var ErrCalendarNotAuthorized = errors.New("calendar_not_authorized")

// ErrCalendarNotConnected is returned when the account has no token.
var ErrCalendarNotConnected = errors.New("google calendar account not connected")

// GoogleCalendarService is one Google account whose calendars frames can show.
// It is the shared Google account with the calendar.readonly scope and the
// Calendar API calls on top; it owns no photos.
type GoogleCalendarService struct {
	*googleAccount
	calendar *gcalendar.Client
}

// NewGoogleCalendarService builds the service for connection id. cfg supplies
// the global OAuth client id and secret with the Calendar scopes.
func NewGoogleCalendarService(conns *ConnectionStore, id uint, cfg googlephotos.ConfigProvider) *GoogleCalendarService {
	return &GoogleCalendarService{
		googleAccount: newGoogleAccount(conns, id, cfg),
		calendar:      gcalendar.NewClient(),
	}
}

// Stop releases the service; nothing runs in the background.
func (s *GoogleCalendarService) Stop() {}

// Calendars lists the account's calendars.
func (s *GoogleCalendarService) Calendars() ([]gcalendar.Calendar, error) {
	if !s.Connected() {
		return nil, ErrCalendarNotConnected
	}
	httpClient, err := s.HTTPClient()
	if err != nil {
		return nil, err
	}
	calendars, err := s.calendar.GetCalendarList(httpClient)
	if err != nil {
		return nil, err
	}
	if calendars == nil {
		return nil, ErrCalendarNotAuthorized
	}
	return calendars, nil
}

// TodayEvents returns today's events of one calendar ("" means the primary
// one) in the given IANA timezone ("" means the server's).
func (s *GoogleCalendarService) TodayEvents(calendarID, timezone string) ([]gcalendar.Event, error) {
	if !s.Connected() {
		return nil, ErrCalendarNotConnected
	}
	httpClient, err := s.HTTPClient()
	if err != nil {
		return nil, err
	}
	if calendarID == "" {
		calendarID = "primary"
	}
	return s.calendar.GetTodayEvents(httpClient, calendarID, timezone)
}
