package gcalendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

type Event struct {
	Summary string    `json:"summary"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	AllDay  bool      `json:"all_day"`
}

type calendarEventsResponse struct {
	Items []calendarEvent `json:"items"`
}

type calendarEvent struct {
	Summary string        `json:"summary"`
	Start   eventDateTime `json:"start"`
	End     eventDateTime `json:"end"`
}

type eventDateTime struct {
	DateTime string `json:"dateTime"` // RFC3339 for timed events
	Date     string `json:"date"`     // YYYY-MM-DD for all-day events
}

type Calendar struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Primary bool   `json:"primary"`
}

type calendarListResponse struct {
	Items []calendarListEntry `json:"items"`
}

type calendarListEntry struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Primary bool   `json:"primary"`
}

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// GetCalendarList fetches the list of calendars for the authenticated user.
// Returns nil, nil on 401/403 (insufficient scopes).
func (c *Client) GetCalendarList(httpClient *http.Client) ([]Calendar, error) {
	resp, err := httpClient.Get("https://www.googleapis.com/calendar/v3/users/me/calendarList")
	if err != nil {
		return nil, fmt.Errorf("calendar list API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar list API returned status %d", resp.StatusCode)
	}

	var result calendarListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode calendar list response: %w", err)
	}

	var calendars []Calendar
	for _, item := range result.Items {
		calendars = append(calendars, Calendar{
			ID:      item.ID,
			Summary: item.Summary,
			Primary: item.Primary,
		})
	}
	return calendars, nil
}

// GetTodayEvents fetches calendar events for today using the provided authenticated HTTP client.
// calendarID specifies which calendar to query (use "primary" for the user's primary calendar).
// timezone is an IANA timezone string (e.g. "Asia/Taipei") used to determine "today" and filter
// past events. If empty, the server's local timezone is used.
// Returns only current and upcoming events sorted by start time. Returns nil (not error) if the
// API call fails due to insufficient scopes, so callers can gracefully degrade.
func (c *Client) GetTodayEvents(httpClient *http.Client, calendarID string, timezone string) ([]Event, error) {
	if calendarID == "" {
		calendarID = "primary"
	}

	// Determine the correct timezone for "today" computation.
	loc := time.Now().Location()
	if timezone != "" {
		if parsed, err := time.LoadLocation(timezone); err == nil {
			loc = parsed
		}
	}

	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	params := url.Values{}
	params.Set("timeMin", startOfDay.Format(time.RFC3339))
	params.Set("timeMax", endOfDay.Format(time.RFC3339))
	params.Set("singleEvents", "true")
	params.Set("orderBy", "startTime")
	params.Set("maxResults", "10")

	apiURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?%s", url.PathEscape(calendarID), params.Encode())

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("calendar API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		// Insufficient scopes or token expired - return empty, not error
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar API returned status %d", resp.StatusCode)
	}

	var result calendarEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode calendar response: %w", err)
	}

	// Parse all-day dates in the device timezone so comparisons are correct.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	tomorrow := today.Add(24 * time.Hour)

	var events []Event
	for _, item := range result.Items {
		ev := Event{Summary: item.Summary}

		if item.Start.DateTime != "" {
			t, err := time.Parse(time.RFC3339, item.Start.DateTime)
			if err == nil {
				ev.Start = t
			}
		} else if item.Start.Date != "" {
			// Parse all-day dates in device timezone
			t, err := time.ParseInLocation("2006-01-02", item.Start.Date, loc)
			if err == nil {
				ev.Start = t
				ev.AllDay = true
			}
		}

		if item.End.DateTime != "" {
			t, err := time.Parse(time.RFC3339, item.End.DateTime)
			if err == nil {
				ev.End = t
			}
		} else if item.End.Date != "" {
			t, err := time.ParseInLocation("2006-01-02", item.End.Date, loc)
			if err == nil {
				ev.End = t
			}
		}

		events = append(events, ev)
	}

	// Filter to only events relevant to today:
	// - All-day events: keep only if they cover today (Start < tomorrow && End > today)
	// - Timed events: keep only if they haven't ended yet
	nowAbs := time.Now()
	var filtered []Event
	for _, ev := range events {
		if ev.AllDay {
			// All-day event spans [Start, End) in date granularity.
			// Keep if it overlaps with today.
			if ev.Start.Before(tomorrow) && ev.End.After(today) {
				filtered = append(filtered, ev)
			}
		} else {
			// Timed event: keep if it hasn't ended
			if ev.End.After(nowAbs) {
				filtered = append(filtered, ev)
			}
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Start.Before(filtered[j].Start)
	})

	return filtered, nil
}

// GetNextEvent returns the closest upcoming event (or currently ongoing) from the given events.
func GetNextEvent(events []Event) *Event {
	now := time.Now()
	for i := range events {
		// Return first event that hasn't ended yet
		if events[i].End.After(now) || events[i].AllDay {
			return &events[i]
		}
	}
	return nil
}

// FormatEventTime returns a human-readable time string for an event.
func FormatEventTime(ev Event) string {
	if ev.AllDay {
		return "All day"
	}
	return ev.Start.Format("15:04")
}
