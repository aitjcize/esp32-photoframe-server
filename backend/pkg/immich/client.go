package immich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is an Immich API client using API key authentication
type Client struct {
	BaseURL        string
	APIKey         string
	httpClient     *http.Client
	downloadClient *http.Client
}

// newTransport creates an HTTP transport that prefers IPv4 to avoid issues
// with .local mDNS hostnames resolving to link-local IPv6 addresses (fe80::)
// which fail without a zone ID.
func newTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Try IPv4 first to avoid link-local IPv6 issues
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// NewClient creates a new Immich client
func NewClient(baseURL, apiKey string) *Client {
	transport := newTransport()
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
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

func (c *Client) do(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	return c.httpClient.Do(req)
}

// TestConnection verifies the server is reachable and the API key is valid
func (c *Client) TestConnection() error {
	resp, err := c.do("GET", "/api/users/me")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status: %d", resp.StatusCode)
	}
	return nil
}

// ListAlbums returns all albums visible to the API key owner
func (c *Client) ListAlbums() ([]Album, error) {
	resp, err := c.do("GET", "/api/albums")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}
	var albums []Album
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, err
	}
	return albums, nil
}

// GetAlbumAssets returns all image assets in the given album
func (c *Client) GetAlbumAssets(albumID string) ([]Asset, error) {
	resp, err := c.do("GET", "/api/albums/"+albumID+"?withAssets=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}
	var album AlbumDetail
	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		return nil, err
	}
	return album.Assets, nil
}

// GetThumbnail fetches thumbnail bytes for an asset.
// size is "thumbnail" (small) or "preview" (large).
func (c *Client) GetThumbnail(assetID, size string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/assets/"+assetID+"/thumbnail?size="+size, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "image/jpeg,image/*,*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("thumbnail fetch returned status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// DownloadOriginal fetches the original full-resolution asset.
func (c *Client) DownloadOriginal(assetID string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/assets/"+assetID+"/original", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("original download returned status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
