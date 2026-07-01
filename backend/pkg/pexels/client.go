// Package pexels is a minimal client for the Pexels public API
// (https://api.pexels.com). An API key is required.
package pexels

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiBase = "https://api.pexels.com/v1"

// Client talks to the Pexels API.
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
	ID     int `json:"id"`
	Width  int `json:"width"`
	Height int `json:"height"`
	Src    struct {
		Original string `json:"original"`
		Large2x  string `json:"large2x"`
		Large    string `json:"large"`
	} `json:"src"`
	Photographer string `json:"photographer"`
}

type searchResponse struct {
	Photos []Photo `json:"photos"`
}

// Search returns one page of photos matching query. perPage is capped at 80
// (the Pexels maximum); page is 1-based.
func (c *Client) Search(query string, perPage, page int) ([]Photo, error) {
	if perPage <= 0 || perPage > 80 {
		perPage = 80
	}
	if page <= 0 {
		page = 1
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("per_page", strconv.Itoa(perPage))
	q.Set("page", strconv.Itoa(page))

	req, err := http.NewRequest(http.MethodGet, apiBase+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels search: status %d", resp.StatusCode)
	}

	var out searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Photos, nil
}
