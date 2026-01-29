package photoframe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	// Custom dialer to force system resolver for mDNS (.local)
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: false,
		},
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
	}
}

// PushImage pushes a PNG image and an optional thumbnail to the device.
func (c *Client) PushImage(host string, pngBytes []byte, thumbBytes []byte) error {
	// Resolve Host to IP manually to bypass HTTP client resolver issues with mDNS
	ip, err := c.resolveHost(host)
	if err != nil {
		return fmt.Errorf("failed to resolve device %s: %w", host, err)
	}

	// Quick reachability check on IP
	if err := c.checkReachability(ip); err != nil {
		return fmt.Errorf("device %s (%s) is not reachable: %w", host, ip, err)
	}

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 1. Add PNG part
	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(pngBytes)); err != nil {
		return fmt.Errorf("failed to copy png bytes: %w", err)
	}

	// 2. Add Thumbnail part (if available)
	if len(thumbBytes) > 0 {
		thumbPart, err := writer.CreateFormFile("thumbnail", "thumbnail.jpg")
		if err != nil {
			return fmt.Errorf("failed to create thumbnail form file: %w", err)
		}
		if _, err := io.Copy(thumbPart, bytes.NewReader(thumbBytes)); err != nil {
			return fmt.Errorf("failed to copy thumbnail bytes: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Construct URL using IP address
	url := fmt.Sprintf("http://%s/api/display-image", ip)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Set Host header just in case, though usually not needed for direct IP
	req.Host = host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) resolveHost(host string) (string, error) {
	// If it's already an IP, return it
	if net.ParseIP(host) != nil {
		return host, nil
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}

	// Prefer IPv4
	for _, ip := range ips {
		if strings.Contains(ip, ".") {
			return ip, nil
		}
	}

	// Fallback to first (likely IPv6)
	if len(ips) > 0 {
		return ips[0], nil
	}

	return "", fmt.Errorf("no IP found for host %s", host)
}

func (c *Client) checkReachability(ip string) error {
	target := ip
	if !strings.Contains(target, ":") {
		target = net.JoinHostPort(target, "80")
	}

	conn, err := net.DialTimeout("tcp4", target, 2*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

type SystemInfo struct {
	DeviceName string `json:"device_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	BoardName  string `json:"board_name"`
}

func (c *Client) FetchSystemInfo(host string) (*SystemInfo, error) {
	ip, err := c.resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device %s: %w", host, err)
	}

	url := fmt.Sprintf("http://%s/api/system-info", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host // Set Host header

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	var info SystemInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode system info: %w", err)
	}

	return &info, nil
}

type ProcessingSettings struct {
	Exposure             float64 `json:"exposure"`
	Saturation           float64 `json:"saturation"`
	ToneMode             string  `json:"toneMode"`
	Contrast             float64 `json:"contrast"`
	Strength             float64 `json:"strength"`
	ShadowBoost          float64 `json:"shadowBoost"`
	HighlightCompress    float64 `json:"highlightCompress"`
	Midpoint             float64 `json:"midpoint"`
	ColorMethod          string  `json:"colorMethod"`
	ProcessingMode       string  `json:"processingMode"`
	DitherAlgorithm      string  `json:"ditherAlgorithm"`
	CompressDynamicRange bool    `json:"compressDynamicRange"`
}

type PaletteColor struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

type Palette struct {
	Black  PaletteColor `json:"black"`
	White  PaletteColor `json:"white"`
	Yellow PaletteColor `json:"yellow"`
	Red    PaletteColor `json:"red"`
	Blue   PaletteColor `json:"blue"`
	Green  PaletteColor `json:"green"`
}

func (c *Client) FetchProcessingSettings(host string) (*ProcessingSettings, error) {
	ip, err := c.resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device %s: %w", host, err)
	}

	url := fmt.Sprintf("http://%s/api/settings/processing", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	var settings ProcessingSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return nil, fmt.Errorf("failed to decode settings: %w", err)
	}

	return &settings, nil
}

type DeviceConfig struct {
	DisplayOrientation string `json:"display_orientation"`
}

func (c *Client) FetchDeviceConfig(host string) (*DeviceConfig, error) {
	ip, err := c.resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device %s: %w", host, err)
	}

	url := fmt.Sprintf("http://%s/api/config", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	var config DeviceConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &config, nil
}

func (c *Client) FetchPalette(host string) (*Palette, error) {
	ip, err := c.resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device %s: %w", host, err)
	}

	url := fmt.Sprintf("http://%s/api/settings/palette", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	var palette Palette
	if err := json.NewDecoder(resp.Body).Decode(&palette); err != nil {
		return nil, fmt.Errorf("failed to decode palette: %w", err)
	}

	return &palette, nil
}
