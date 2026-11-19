package nextcloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/mdns"
)

// UserAgent identifies this server to Nextcloud. Login Flow v2 names the app
// password it creates after it, so it is what the user sees under Personal
// settings → Security → Devices & sessions.
const UserAgent = "ESP32 PhotoFrame Server"

// LoginFlow is a started Nextcloud Login Flow v2: the page the user opens to
// sign in and grant access, and the token this server polls with.
type LoginFlow struct {
	LoginURL string
	Token    string
}

// Credentials is what a completed login flow hands over: the user's login
// name and an app password created for this server.
type Credentials struct {
	Server      string `json:"server"`
	LoginName   string `json:"loginName"`
	AppPassword string `json:"appPassword"`
}

// NewLoginHTTPClient is the HTTP client for the login flow (mDNS-aware, like
// the API client).
func NewLoginHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: mdns.NewTransport()}
}

func postForm(hc *http.Client, target string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequest("POST", target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	return hc.Do(req)
}

// StartLogin begins Login Flow v2 on the Nextcloud at baseURL.
func StartLogin(hc *http.Client, baseURL string) (*LoginFlow, error) {
	base := strings.TrimSuffix(baseURL, "/")
	resp, err := postForm(hc, base+"/index.php/login/v2", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("Nextcloud login flow returned status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var body struct {
		Poll struct {
			Token string `json:"token"`
		} `json:"poll"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("unexpected login flow response (is this a Nextcloud URL?): %w", err)
	}
	if body.Login == "" || body.Poll.Token == "" {
		return nil, fmt.Errorf("unexpected login flow response (is this a Nextcloud URL?)")
	}
	return &LoginFlow{LoginURL: body.Login, Token: body.Poll.Token}, nil
}

// PollLogin asks whether the user has granted access yet: nil credentials
// (and no error) while the flow is still pending. The poll goes to baseURL
// rather than the endpoint Nextcloud names, which a reverse proxy can leave
// pointing at an address this server cannot reach.
func PollLogin(hc *http.Client, baseURL, token string) (*Credentials, error) {
	base := strings.TrimSuffix(baseURL, "/")
	resp, err := postForm(hc, base+"/index.php/login/v2/poll", url.Values{"token": {token}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, nil // not granted yet (or the token expired)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("login poll returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var creds Credentials
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil, err
	}
	if creds.LoginName == "" || creds.AppPassword == "" {
		return nil, fmt.Errorf("login poll returned no credentials")
	}
	return &creds, nil
}

// RevokeAppPassword deletes the app password this client authenticates
// with, so it disappears from the user's Devices & sessions.
func (c *Client) RevokeAppPassword() error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/ocs/v2.php/core/apppassword", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.Username, c.AppPassword)
	req.Header.Set("OCS-APIRequest", "true")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return statusError(resp, "app password revocation")
}
