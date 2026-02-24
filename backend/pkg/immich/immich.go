package immich

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client communicates with an Immich server.
type Client struct {
	httpClient *http.Client
}

// Album represents an Immich album.
type Album struct {
	ID         string `json:"id"`
	AlbumName  string `json:"albumName"`
	AssetCount int    `json:"assetCount"`
}

// Asset represents an Immich asset (photo/video).
type Asset struct {
	ID               string `json:"id"`
	Type             string `json:"type"` // "IMAGE" or "VIDEO"
	OriginalFileName string `json:"originalFileName"`
	OriginalMimeType string `json:"originalMimeType"`
}

// NewClient creates a new Immich API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an authenticated request to the Immich API.
func (c *Client) doRequest(method, serverURL, apiKey, path string, body io.Reader) (*http.Response, error) {
	url := strings.TrimRight(serverURL, "/") + "/api" + path

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// TestConnection verifies that we can connect to the Immich server with the given credentials.
func (c *Client) TestConnection(serverURL, apiKey string) error {
	resp, err := c.doRequest("GET", serverURL, apiKey, "/server/ping", nil)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ListAlbums returns all albums from the Immich server.
func (c *Client) ListAlbums(serverURL, apiKey string) ([]Album, error) {
	resp, err := c.doRequest("GET", serverURL, apiKey, "/albums", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list albums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var albums []Album
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, fmt.Errorf("failed to decode albums: %w", err)
	}

	return albums, nil
}

// GetRandomAsset fetches a random image asset from the Immich server.
// If albumID is provided, filters to that album. Only returns IMAGE type assets.
func (c *Client) GetRandomAsset(serverURL, apiKey, albumID string) (*Asset, error) {
	// Build the search/random request body
	reqBody := map[string]interface{}{
		"size": 1,
		"type": "IMAGE",
	}
	if albumID != "" {
		reqBody["albumIds"] = []string{albumID}
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest("POST", serverURL, apiKey, "/search/random", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to search random: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var assets []Asset
	if err := json.NewDecoder(resp.Body).Decode(&assets); err != nil {
		return nil, fmt.Errorf("failed to decode assets: %w", err)
	}

	if len(assets) == 0 {
		return nil, fmt.Errorf("no assets found")
	}

	return &assets[0], nil
}

// DownloadAsset downloads the original file for an asset.
func (c *Client) DownloadAsset(serverURL, apiKey, assetID string) ([]byte, error) {
	url := strings.TrimRight(serverURL, "/") + "/api/assets/" + assetID + "/original"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)

	// Use a longer timeout for downloads
	downloadClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read asset data: %w", err)
	}

	return data, nil
}
