// Package unsplash is a minimal client for the Unsplash public API
// (https://api.unsplash.com). A Client-ID access key is required.
package unsplash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiBase = "https://api.unsplash.com"

// Client talks to the Unsplash API.
type Client struct {
	APIKey string
	HTTP   *http.Client
}

// New constructs a client with a sane timeout.
func New(apiKey string) *Client {
	return &Client{APIKey: apiKey, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Photo is one search result (only the fields we use).
type Photo struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	URLs   struct {
		Raw     string `json:"raw"`
		Regular string `json:"regular"`
	} `json:"urls"`
	Links struct {
		DownloadLocation string `json:"download_location"`
	} `json:"links"`
	User struct {
		Name string `json:"name"`
	} `json:"user"`
}

type searchResponse struct {
	Results []Photo `json:"results"`
	Total   int     `json:"total"`
}

// Search returns one page of photos matching query, plus the total number of
// results the query has. perPage is capped at 30 (the Unsplash maximum); page
// is 1-based.
func (c *Client) Search(query string, perPage, page int) ([]Photo, int, error) {
	if perPage <= 0 || perPage > 30 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("per_page", strconv.Itoa(perPage))
	q.Set("page", strconv.Itoa(page))
	q.Set("content_filter", "high")

	req, err := http.NewRequest(http.MethodGet, apiBase+"/search/photos?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Client-ID "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unsplash search: status %d", resp.StatusCode)
	}

	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	return out.Results, out.Total, nil
}
