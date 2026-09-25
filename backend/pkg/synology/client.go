package synology

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/mdns"
)

type Client struct {
	BaseURL    string
	Account    string
	Password   string
	httpClient *http.Client
	SID        string
	DID        string
	SynoToken  string
}

func NewClient(baseURL, account, password string, insecure bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := mdns.NewTransport()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &Client{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Account:  account,
		Password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
			Jar:       jar,
		},
	}, nil
}

// Jar returns the cookie jar for setting cookies manually
func (c *Client) Jar() http.CookieJar {
	return c.httpClient.Jar
}

type AuthResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Sid       string `json:"sid"`
		Synotoken string `json:"synotoken"`
	} `json:"data"`
	Error struct {
		Code int `json:"code"`
	} `json:"error"`
}

func (c *Client) Login(otpCode string) error {
	endpoint := fmt.Sprintf("%s/webapi/auth.cgi", c.BaseURL)

	params := url.Values{}
	params.Set("api", "SYNO.API.Auth")
	params.Set("version", "3")
	params.Set("method", "login")
	params.Set("account", c.Account)
	params.Set("passwd", c.Password)
	params.Set("session", "PhotoFrame")
	params.Set("format", "cookie")
	params.Set("enable_device_token", "yes")
	params.Set("device_name", "PhotoFrame")
	params.Set("enable_syno_token", "yes")
	params.Set("logintype", "local")
	params.Set("rememberme", "1")

	if otpCode != "" {
		params.Set("otp_code", otpCode)
	}

	resp, err := c.httpClient.Get(endpoint + "?" + params.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth returned status: %d", resp.StatusCode)
	}

	// Capture id and did cookies
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "id" {
			c.SID = cookie.Value
		} else if cookie.Name == "did" {
			c.DID = cookie.Value
		}
	}

	// Capture SynoToken from header
	if token := resp.Header.Get("X-SYNO-TOKEN"); token != "" {
		c.SynoToken = token
	}

	var result AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("login failed with code: %d", result.Error.Code)
	}

	if result.Data.Sid != "" {
		c.SID = result.Data.Sid
	}
	if result.Data.Synotoken != "" {
		c.SynoToken = result.Data.Synotoken
	}

	return nil
}

func (c *Client) Logout() error {
	if c.SID == "" {
		return nil
	}
	endpoint := fmt.Sprintf("%s/webapi/auth.cgi", c.BaseURL)
	params := url.Values{}
	params.Set("api", "SYNO.API.Auth")
	params.Set("version", "3")
	c.httpClient.Get(endpoint + "?" + params.Encode())
	c.SID = ""
	return nil
}

func (c *Client) ListAlbums(offset, limit int) ([]Album, error) {
	endpoint := fmt.Sprintf("%s/webapi/entry.cgi", c.BaseURL)
	params := url.Values{}
	params.Set("api", "SYNO.Foto.Browse.Album")
	params.Set("version", "1")
	params.Set("method", "list")
	params.Set("type", "album")
	params.Set("offset", fmt.Sprintf("%d", offset))
	params.Set("limit", fmt.Sprintf("%d", limit))
	if c.SynoToken != "" {
		params.Set("SynoToken", c.SynoToken)
	}

	req, err := http.NewRequest("GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var result BrowseAlbumResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("api call failed with code: %d", result.Error.Code)
	}

	return result.Data.List, nil
}

// ListSharedWithMeAlbums returns the albums other DSM users have shared with
// the logged-in account. SYNO.Foto.Browse.Album lists only albums the account
// owns, which left shared albums invisible to the album picker — issue #52.
func (c *Client) ListSharedWithMeAlbums(offset, limit int) ([]Album, error) {
	endpoint := fmt.Sprintf("%s/webapi/entry.cgi", c.BaseURL)
	params := url.Values{}
	params.Set("api", "SYNO.Foto.Sharing.Misc")
	params.Set("version", "1")
	// DSM 7.4 names the album listing list_shared_with_me_album;
	// list_shared_with_me does not exist and returns error 103.
	params.Set("method", "list_shared_with_me_album")
	params.Set("offset", fmt.Sprintf("%d", offset))
	params.Set("limit", fmt.Sprintf("%d", limit))
	if c.SynoToken != "" {
		params.Set("SynoToken", c.SynoToken)
	}

	req, err := http.NewRequest("GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var result BrowseAlbumResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("api call failed with code: %d", result.Error.Code)
	}

	return result.Data.List, nil
}

func (c *Client) ListPhotos(offset, limit int, album AlbumRef) ([]Item, error) {
	endpoint := fmt.Sprintf("%s/webapi/entry.cgi", c.BaseURL)
	params := url.Values{}
	params.Set("api", "SYNO.Foto.Browse.Item")
	params.Set("version", "1")
	params.Set("method", "list")
	params.Set("type", "photo")
	params.Set("offset", fmt.Sprintf("%d", offset))
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("additional", `["thumbnail","resolution"]`)
	// Shared albums are browsed through the passphrase DSM issued for them.
	// DSM rejects album_id and passphrase together (error 120), so the
	// passphrase replaces the album id rather than accompanying it.
	if album.Passphrase != "" {
		params.Set("passphrase", album.Passphrase)
	} else if album.ID != 0 {
		params.Set("album_id", fmt.Sprintf("%d", album.ID))
	}
	if c.SynoToken != "" {
		params.Set("SynoToken", c.SynoToken)
	}

	req, err := http.NewRequest("GET", endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var result BrowseItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("api call failed with code: %d", result.Error.Code)
	}

	return result.Data.List, nil
}

// GetPhoto fetches a thumbnail
// size: "small", "medium", "large"
func (c *Client) GetPhoto(id int, cacheKey string, size string, album AlbumRef, synoToken string) ([]byte, error) {
	path := "/webapi/entry.cgi"
	fullURL, _ := url.JoinPath(c.BaseURL, path)

	api := "SYNO.Foto.Thumbnail"

	// Map size
	sz := "xl"
	switch size {
	case "small":
		sz = "sm"
	case "medium":
		sz = "m"
	case "large":
		sz = "xl"
	}

	// Strictly match parameter order from curl:
	// id=3253&cache_key=%22...%22&type=%22item%22&size=%22xl%22&album_id=21&api=%22...%22&method=%22get%22&version=2&SynoToken=...
	//
	// id is the item ID, so type must be "item". type "unit" expects the
	// thumbnail's unit_id, which only happens to equal the item ID for older
	// items; for the rest DSM answers 404.
	parts := []string{
		fmt.Sprintf("id=%d", id),
		fmt.Sprintf("cache_key=%s", url.QueryEscape(fmt.Sprintf("\"%s\"", cacheKey))),
		"type=%22item%22",
		fmt.Sprintf("size=%s", url.QueryEscape(fmt.Sprintf("\"%s\"", sz))),
	}
	// As in ListPhotos: the passphrase replaces album_id; with both set DSM
	// answers 404.
	if album.Passphrase != "" {
		parts = append(parts, fmt.Sprintf("passphrase=%s",
			url.QueryEscape(fmt.Sprintf("\"%s\"", album.Passphrase))))
	} else if album.ID != 0 {
		parts = append(parts, fmt.Sprintf("album_id=%d", album.ID))
	}
	parts = append(parts,
		fmt.Sprintf("api=%s", url.QueryEscape(fmt.Sprintf("\"%s\"", api))),
		"method=%22get%22",
		"version=2",
	)
	if synoToken != "" {
		parts = append(parts, fmt.Sprintf("SynoToken=%s", synoToken))
	}

	reqURL := fullURL + "?" + strings.Join(parts, "&")
	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status: %d (URL: %s)", resp.StatusCode, reqURL)
	}

	return io.ReadAll(resp.Body)
}

// DownloadPhoto fetches the full image using the Download API
func (c *Client) DownloadPhoto(id int) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/webapi/entry.cgi", c.BaseURL)
	api := "SYNO.Foto.Download"

	params := url.Values{}
	params.Set("api", api)
	params.Set("version", "1")
	params.Set("method", "download")
	params.Set("item_id", fmt.Sprintf("[%d]", id))
	params.Set("force_download", "true")

	resp, err := c.httpClient.PostForm(endpoint, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status: %d (URL: %s)", resp.StatusCode, endpoint)
	}

	return io.ReadAll(resp.Body)
}
