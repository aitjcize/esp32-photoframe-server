// Package artic is a minimal client for the Art Institute of Chicago public API
// (https://api.artic.edu). No API key is required. Images are delivered via IIIF.
package artic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	apiBase = "https://api.artic.edu/api/v1"
	// iiifBase is the IIIF image endpoint; artwork images are fetched as
	// {iiifBase}/{image_id}/full/{width},/0/default.jpg.
	iiifBase = "https://www.artic.edu/iiif/2"
	// ImageWidth is the IIIF-scaled width we request; large enough for any
	// panel, the render pipeline downsizes to the device.
	ImageWidth = 1920
)

// Client talks to the ARTIC API.
type Client struct {
	HTTP *http.Client
}

// New constructs a client with a sane timeout.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Artwork is one search result (only the fields we use).
type Artwork struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ArtistTitle string `json:"artist_title"`
	ImageID     string `json:"image_id"`
	Thumbnail   struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"thumbnail"`
}

// ImageURL returns the IIIF URL for this artwork at the standard width.
func (a Artwork) ImageURL() string {
	return fmt.Sprintf("%s/%s/full/%d,/0/default.jpg", iiifBase, a.ImageID, ImageWidth)
}

type searchResponse struct {
	Data []Artwork `json:"data"`
}

// Search returns up to limit public-domain artworks matching query. ARTIC caps
// limit at 100 per request; callers wanting more should page, but for a topic
// feed one page of 100 is plenty.
func (c *Client) Search(query string, limit int) ([]Artwork, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("query[term][is_public_domain]", "true")
	q.Set("fields", "id,title,artist_title,image_id,thumbnail")
	q.Set("limit", strconv.Itoa(limit))

	req, err := http.NewRequest(http.MethodGet, apiBase+"/artworks/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("AIC-User-Agent", "esp32-photoframe (github.com/aitjcize/esp32-photoframe-server)")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artic search: status %d", resp.StatusCode)
	}

	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	// Drop results without an image.
	kept := out.Data[:0]
	for _, a := range out.Data {
		if a.ImageID != "" {
			kept = append(kept, a)
		}
	}
	return kept, nil
}
