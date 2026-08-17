package immich

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/mdns"
)

// Client is an Immich API client using API key authentication
type Client struct {
	BaseURL        string
	APIKey         string
	httpClient     *http.Client
	downloadClient *http.Client
}

// NewClient creates a new Immich client
func NewClient(baseURL, apiKey string) *Client {
	transport := mdns.NewTransport()
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

// GetAlbumAssets returns all image assets in the given album.
//
// Immich v2 embeds the asset array in GET /api/albums/{id}?withAssets=true.
// Immich v3 (>= 3.0) dropped that array from the album response — only
// assetCount remains — so we fall back to POST /api/search/metadata scoped by
// albumIds when the response has assets missing but a non-zero count. That
// keeps a single code path working across both server versions.
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
	if len(album.Assets) > 0 || album.AssetCount == 0 {
		return album.Assets, nil // v2, or a genuinely empty album
	}
	// v3: the album has assets but they weren't inlined — page them via search.
	return c.SearchAssets(SearchMetadataRequest{AlbumIDs: []string{albumID}})
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

// doJSON is like do() but for POST bodies with a JSON payload.
func (c *Client) doJSON(method, path string, body interface{}) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, c.BaseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// SearchAssets pages through POST /api/search/metadata until the server runs
// out of results, returning every IMAGE asset that matched. Use the
// pre-filled SearchMetadataRequest to pick a filter mode (favorites,
// date-bound, etc.); the function fills in Type, Page, and Size itself.
func (c *Client) SearchAssets(filter SearchMetadataRequest) ([]Asset, error) {
	const pageSize = 250
	filter.Type = "IMAGE"
	filter.Size = pageSize
	filter.WithExif = true // v3 omits exifInfo unless asked; v2 ignores the flag

	var out []Asset
	for page := 1; ; page++ {
		filter.Page = page
		resp, err := c.doJSON("POST", "/api/search/metadata", filter)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(b))
		}
		var body searchAssetsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		out = append(out, body.Assets.Items...)
		if len(body.Assets.Items) < pageSize || body.Assets.NextPage == nil {
			break
		}
	}
	return out, nil
}

// GetMemoryAssets returns "on this day" assets — Immich returns one
// MemoryLane per past year that has a photo from this month/day.
//
// The /api/memories endpoint must be scoped with a `for` date, otherwise
// Immich returns every persisted memory lane the user has rather than the
// ones relevant to today. We pass today's date (UTC) plus type=on_this_day
// so the frame shows "this day, past years" instead of a random grab-bag.
//
// When latestYearOnly is true, only the most recent year's lane is returned
// (a focused "last year on this day" experience); otherwise every lane is
// flattened into one pool so the frame shuffles across all years.
func (c *Client) GetMemoryAssets(latestYearOnly bool) ([]Asset, error) {
	q := url.Values{}
	q.Set("for", time.Now().UTC().Format("2006-01-02"))
	q.Set("type", "on_this_day")
	resp, err := c.do("GET", "/api/memories?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memories returned status %d: %s", resp.StatusCode, string(b))
	}
	var lanes []MemoryLane
	if err := json.NewDecoder(resp.Body).Decode(&lanes); err != nil {
		return nil, err
	}

	if latestYearOnly {
		if len(lanes) == 0 {
			return nil, nil
		}
		best := lanes[0]
		for _, lane := range lanes[1:] {
			if lane.Data.Year > best.Data.Year {
				best = lane
			}
		}
		return best.Assets, nil
	}

	var out []Asset
	for _, lane := range lanes {
		out = append(out, lane.Assets...)
	}
	return out, nil
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
