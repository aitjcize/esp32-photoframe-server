// Package nextcloud is a client for the Memories app on a Nextcloud server:
// its album list, its day-grouped timeline, and its image endpoints.
//
// Requests authenticate with HTTP Basic (user name + app password), the way
// the Memories mobile app does, and carry OCS-APIRequest so Nextcloud treats
// them as API calls rather than browser requests.
package nextcloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/mdns"
)

// memoriesAPI is the Memories app's API root, below the server URL.
const memoriesAPI = "/index.php/apps/memories/api"

// dayBatch is how many day ids one GET /api/days/{ids} request carries.
const dayBatch = 50

// ErrUnauthorized is returned when Nextcloud rejects the credentials.
var ErrUnauthorized = errors.New("invalid username or app password")

// ErrMemoriesMissing is returned when the server answers but the Memories app
// is not installed or not enabled for the user.
var ErrMemoriesMissing = errors.New("the Memories app is not installed or not enabled on this Nextcloud")

// Client talks to one Nextcloud server as one user.
type Client struct {
	BaseURL        string
	Username       string
	AppPassword    string
	httpClient     *http.Client
	downloadClient *http.Client
}

// NewClient creates a client for baseURL (the Nextcloud root, e.g.
// https://cloud.example.com or https://example.com/nextcloud).
func NewClient(baseURL, username, appPassword string) *Client {
	transport := mdns.NewTransport()
	return &Client{
		BaseURL:     strings.TrimSuffix(baseURL, "/"),
		Username:    username,
		AppPassword: appPassword,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		downloadClient: &http.Client{
			Timeout:   2 * time.Minute,
			Transport: transport,
		},
	}
}

func (c *Client) newRequest(path, accept string) (*http.Request, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.AppPassword)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", UserAgent)
	return req, nil
}

// getJSON GETs path and decodes the JSON response into out.
func (c *Client) getJSON(path string, out any) error {
	req, err := c.newRequest(path, "application/json")
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := statusError(resp, "request"); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// statusError maps a non-200 response to an error.
func statusError(resp *http.Response, what string) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("%s returned status %d: %s", what, resp.StatusCode, strings.TrimSpace(string(body)))
}

// TestConnection checks that the credentials are accepted and that the
// Memories app answers for this user.
func (c *Client) TestConnection() error {
	var user struct{}
	if err := c.getJSON("/ocs/v2.php/cloud/user?format=json", &user); err != nil {
		return err
	}
	req, err := c.newRequest(memoriesAPI+"/clusters/albums", "application/json")
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrMemoriesMissing
	}
	return statusError(resp, "Memories album list")
}

// ListAlbums returns the albums the user owns and those shared with them.
func (c *Client) ListAlbums() ([]Album, error) {
	var albums []Album
	if err := c.getJSON(memoriesAPI+"/clusters/albums", &albums); err != nil {
		return nil, err
	}
	return albums, nil
}

func (f Filter) query() string {
	q := url.Values{}
	if f.Album != "" {
		q.Set("albums", f.Album)
	}
	if f.Favorites {
		q.Set("fav", "1")
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// GetDays returns the days that have photos matching filter.
func (c *Client) GetDays(f Filter) ([]Day, error) {
	var days []Day
	if err := c.getJSON(memoriesAPI+"/days"+f.query(), &days); err != nil {
		return nil, err
	}
	return days, nil
}

// GetDayPhotos returns the photos matching filter on the given days, a batch
// of days per request.
func (c *Client) GetDayPhotos(f Filter, dayIDs []int64) ([]Photo, error) {
	var out []Photo
	for start := 0; start < len(dayIDs); start += dayBatch {
		end := start + dayBatch
		if end > len(dayIDs) {
			end = len(dayIDs)
		}
		ids := make([]string, 0, end-start)
		for _, id := range dayIDs[start:end] {
			ids = append(ids, strconv.FormatInt(id, 10))
		}
		var photos []Photo
		if err := c.getJSON(memoriesAPI+"/days/"+strings.Join(ids, ",")+f.query(), &photos); err != nil {
			return nil, err
		}
		out = append(out, photos...)
	}
	return out, nil
}

// GetPhotos returns every photo matching filter: the album's photos, the
// favorites, or the whole timeline.
func (c *Client) GetPhotos(f Filter) ([]Photo, error) {
	days, err := c.GetDays(f)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(days))
	for _, d := range days {
		if d.Count > 0 {
			ids = append(ids, d.DayID)
		}
	}
	return c.GetDayPhotos(f, ids)
}

// DayID is the Memories day id of a calendar date: whole days since the
// Unix epoch.
func DayID(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// onThisDayIDs returns the day ids of today's month and day in each of the
// past maxYears years, newest first. Feb 29 only exists in leap years; other
// years are skipped rather than rolled over to Mar 1.
func onThisDayIDs(now time.Time, maxYears int) []int64 {
	y, m, d := now.Date()
	out := make([]int64, 0, maxYears)
	for i := 1; i <= maxYears; i++ {
		t := time.Date(y-i, m, d, 0, 0, 0, 0, time.UTC)
		if t.Month() != m || t.Day() != d {
			continue
		}
		out = append(out, t.Unix()/86400)
	}
	return out
}

// GetOnThisDay returns the photos taken on today's date (local time, like
// Memories' own "On this day") in past years. With latestYearOnly, only the
// most recent year that has a servable image is returned; otherwise every
// year is flattened into one pool.
func (c *Client) GetOnThisDay(latestYearOnly bool, now time.Time) ([]Photo, error) {
	days, err := c.GetDays(Filter{})
	if err != nil {
		return nil, err
	}
	has := make(map[int64]bool, len(days))
	for _, d := range days {
		if d.Count > 0 {
			has[d.DayID] = true
		}
	}
	var ids []int64
	for _, id := range onThisDayIDs(now, 120) {
		if has[id] {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	photos, err := c.GetDayPhotos(Filter{}, ids)
	if err != nil {
		return nil, err
	}
	if !latestYearOnly {
		return photos, nil
	}
	byDay := map[int64][]Photo{}
	for _, p := range photos {
		byDay[p.DayID] = append(byDay[p.DayID], p)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	for _, id := range ids {
		for _, p := range byDay[id] {
			if IsServableImage(p) {
				return byDay[id], nil
			}
		}
	}
	return nil, nil
}

// IsServableImage reports whether a photo can be shown on the frame: an
// image (not a video, not a hidden live-photo part) that isn't a RAW file.
func IsServableImage(p Photo) bool {
	if p.FileID == 0 || bool(p.IsVideo) || bool(p.IsHidden) {
		return false
	}
	if p.Mimetype != "" && !strings.HasPrefix(p.Mimetype, "image/") {
		return false
	}
	switch strings.ToLower(filepath.Ext(p.Basename)) {
	case ".dng", ".cr2", ".cr3", ".nef", ".arw", ".raf", ".orf", ".rw2":
		return false
	}
	return true
}

// TakenAt dates a photo: its taken timestamp when the server sent one,
// otherwise the start of its day.
func (p Photo) TakenAt() *time.Time {
	var t time.Time
	switch {
	case p.DateTaken > 0:
		t = time.Unix(p.DateTaken, 0).UTC()
	case p.DayID > 0:
		t = time.Unix(p.DayID*86400, 0).UTC()
	default:
		return nil
	}
	return &t
}

// fetchBytes GETs an image path with the given client.
func (c *Client) fetchBytes(hc *http.Client, path, what string) ([]byte, error) {
	req, err := c.newRequest(path, "image/*,*/*")
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := statusError(resp, what); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// GetPreview returns a preview of the file fitting within size×size,
// aspect ratio kept.
func (c *Client) GetPreview(fileID int64, size int) ([]byte, error) {
	q := url.Values{}
	q.Set("x", strconv.Itoa(size))
	q.Set("y", strconv.Itoa(size))
	q.Set("a", "1")
	return c.fetchBytes(c.httpClient,
		fmt.Sprintf("%s/image/preview/%d?%s", memoriesAPI, fileID, q.Encode()), "preview")
}

// DownloadDecodable returns the full image in a format browsers can decode:
// the original for JPEG/PNG, a server-side JPEG conversion for HEIC and
// similar.
func (c *Client) DownloadDecodable(fileID int64) ([]byte, error) {
	return c.fetchBytes(c.downloadClient,
		fmt.Sprintf("%s/image/decodable/%d", memoriesAPI, fileID), "image download")
}
