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
	return c.fetchAssetBytes(c.httpClient,
		"/api/assets/"+assetID+"/thumbnail?size="+size, "thumbnail fetch")
}

// fetchAssetBytes GETs an asset-serving path with edited=true appended, so
// Immich returns the edited rendition of an asset when one exists (crops etc.
// made in the Immich editor, v2.5.0+) and the untouched file otherwise — see
// issue #46. Both /original and /thumbnail default to edited=false, which is
// why edits never reached the frame. If a server rejects the parameter with a
// 400 (in case some version validates its query strictly), the request is
// retried without it.
func (c *Client) fetchAssetBytes(httpClient *http.Client, path, what string) ([]byte, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	data, status, err := c.fetchBytes(httpClient, path+sep+"edited=true", what)
	if status == http.StatusBadRequest {
		data, _, err = c.fetchBytes(httpClient, path, what)
	}
	return data, err
}

func (c *Client) fetchBytes(httpClient *http.Client, path, what string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/octet-stream,image/*,*/*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("%s returned status %d: %s", what, resp.StatusCode, string(body))
	}
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
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

// legacyMemoriesForLayout is the full-timestamp format Immich v3.0.0–v3.0.2
// require for the memories `for` param (validated as an ISO datetime there);
// v3.0.3+ validates it as a strict YYYY-MM-DD date and v2.x accepts both.
const legacyMemoriesForLayout = "2006-01-02T15:04:05.000Z"

// GetMemoryAssets returns "on this day" assets — Immich returns one
// MemoryLane per past year that has a photo from this month/day.
//
// The /api/memories endpoint must be scoped with a `for` date, otherwise
// Immich returns every persisted memory lane the user has rather than the
// ones relevant to today. We pass today's date plus type=on_this_day so the
// frame shows "this day, past years" instead of a random grab-bag. The date
// is local (like the official web client), so the lane flips at local
// midnight; date-only is tried first (required by Immich v3.0.3+) with a
// retry in the legacy timestamp format for v3.0.0–v3.0.2 on a 400.
//
// When latestYearOnly is true, only the most recent year's lane is returned
// (a focused "last year on this day" experience); otherwise every lane is
// flattened into one pool so the frame shuffles across all years.
func (c *Client) GetMemoryAssets(latestYearOnly bool) ([]Asset, error) {
	now := time.Now()
	lanes, status, err := c.getMemoryLanes(now.Format(time.DateOnly))
	if status == http.StatusBadRequest {
		lanes, _, err = c.getMemoryLanes(now.UTC().Format(legacyMemoriesForLayout))
	}
	if err != nil {
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

// getMemoryLanes performs one GET /api/memories request scoped to forDate.
// The HTTP status is returned alongside the error so the caller can
// distinguish a validation reject (retryable with another date format) from
// other failures.
func (c *Client) getMemoryLanes(forDate string) ([]MemoryLane, int, error) {
	q := url.Values{}
	q.Set("for", forDate)
	q.Set("type", "on_this_day")
	resp, err := c.do("GET", "/api/memories?"+q.Encode())
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("memories returned status %d: %s", resp.StatusCode, string(b))
	}
	var lanes []MemoryLane
	if err := json.NewDecoder(resp.Body).Decode(&lanes); err != nil {
		return nil, resp.StatusCode, err
	}
	return lanes, resp.StatusCode, nil
}

// DownloadOriginal fetches the full-resolution asset — the edited rendition
// when one exists, the untouched original otherwise.
func (c *Client) DownloadOriginal(assetID string) ([]byte, error) {
	return c.fetchAssetBytes(c.downloadClient,
		"/api/assets/"+assetID+"/original", "original download")
}
