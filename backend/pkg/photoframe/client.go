package photoframe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// MinEPDGZVersion is the minimum firmware version that supports epdgz format.
const MinEPDGZVersion = "2.6.1"

// SupportsEPDGZ returns true if the given firmware version supports the epdgz
// format. Dev builds (dev-<hash>) run the latest code, so they're treated as
// supporting it -- otherwise compareVersions ranks every dev build below the
// threshold and they'd be stuck on the PNG fallback.
func SupportsEPDGZ(version string) bool {
	if strings.HasPrefix(strings.TrimPrefix(version, "v"), "dev-") {
		return true
	}
	return compareVersions(version, MinEPDGZVersion) > 0
}

// compareVersions compares two semver strings (with optional "v" prefix).
// Returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2.
// Dev versions (e.g. "dev-abc123") are considered older than any release.
func compareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	if strings.HasPrefix(v1, "dev-") {
		return -1
	}
	if strings.HasPrefix(v2, "dev-") {
		return 1
	}

	p1 := parseVersion(v1)
	p2 := parseVersion(v2)

	for i := 0; i < 3; i++ {
		if p1[i] < p2[i] {
			return -1
		}
		if p1[i] > p2[i] {
			return 1
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	var parts [3]int
	segs := strings.SplitN(v, ".", 3)
	for i, s := range segs {
		if i < 3 {
			parts[i], _ = strconv.Atoi(s)
		}
	}
	return parts
}

// Shared HTTP client with mDNS-compatible resolver (reused across all Client instances)
var sharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  &net.Resolver{PreferGo: false},
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	Timeout: 120 * time.Second,
}

type Client struct {
	host       string
	resolvedIP string // Cached resolved IP
	httpClient *http.Client
}

func NewClient(host string) *Client {
	return &Client{
		host:       host,
		httpClient: sharedHTTPClient,
	}
}

// PushImage pushes an EPDGZ image and an optional thumbnail to the device.
func (c *Client) PushImage(imageBytes []byte, thumbBytes []byte) error {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	// Quick reachability check on IP
	if err := c.checkReachability(ip); err != nil {
		return fmt.Errorf("device %s (%s) is not reachable: %w", c.host, ip, err)
	}

	// Prepare multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 1. Add image part
	part, err := writer.CreateFormFile("image", "image.epdgz")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(imageBytes)); err != nil {
		return fmt.Errorf("failed to copy image bytes: %w", err)
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
	req.Host = c.host

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

// Host returns the client's target host.
func (c *Client) Host() string {
	return c.host
}

func (c *Client) resolveHost(host string) (string, error) {
	// Return cached result
	if c.resolvedIP != "" {
		return c.resolvedIP, nil
	}

	// If it's already an IP, cache and return it
	if net.ParseIP(host) != nil {
		c.resolvedIP = host
		return host, nil
	}

	// For .local (mDNS) on macOS, use dns-sd for fast resolution
	// (Go's net.LookupHost has a 5s timeout trying regular DNS first)
	if strings.HasSuffix(host, ".local") && runtime.GOOS == "darwin" {
		if ip, err := resolveMDNSDarwin(host); err == nil {
			c.resolvedIP = ip
			return ip, nil
		}
		// Fall through to standard resolver
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}

	// Prefer IPv4
	for _, ip := range ips {
		if strings.Contains(ip, ".") {
			c.resolvedIP = ip
			return ip, nil
		}
	}

	// Fallback to first (likely IPv6)
	if len(ips) > 0 {
		c.resolvedIP = ips[0]
		return ips[0], nil
	}

	return "", fmt.Errorf("no IP found for host %s", host)
}

// resolveMDNSDarwin uses macOS dns-sd for fast mDNS resolution (~10ms vs 5s).
func resolveMDNSDarwin(host string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "dns-sd", "-G", "v4", host)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip header lines; look for the result line containing the hostname
		if strings.Contains(line, host) && !strings.HasPrefix(line, "DATE") && !strings.HasPrefix(line, "Timestamp") {
			for _, field := range strings.Fields(line) {
				if net.ParseIP(field) != nil && strings.Contains(field, ".") {
					log.Printf("mDNS resolved %s -> %s", host, field)
					cmd.Process.Kill()
					return field, nil
				}
			}
		}
	}

	cmd.Process.Kill()
	cmd.Wait()
	return "", fmt.Errorf("dns-sd: no result for %s", host)
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
	Version    string `json:"version"`
	// Display color model: "spectra6" (6-color), "gc16" (16-level grayscale),
	// or "" for legacy firmware (treated as spectra6).
	DisplayType string `json:"display_type"`
}

func (c *Client) FetchSystemInfo() (*SystemInfo, error) {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/system-info", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Host = c.host

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
	// Grays, when present, marks a grayscale (GC16) palette: an ordered ramp of
	// [r,g,b] levels (0=black..N-1=white). Set instead of the named colors for
	// IT8951 panels; the named fields are ignored when Grays is non-empty.
	Grays [][]int `json:"grays,omitempty"`
	// BlackY / WhiteY, when present, are the panel's measured RELATIVE LUMINANCE
	// (Y, 0..1) of full black / full white -- the calibration for a grayscale
	// (GC16) panel. The 16-level ramp is derived from these two endpoints
	// downstream (epaper-image-convert --gray-black-y/--gray-white-y), so a
	// calibrated grayscale palette is just two numbers.
	BlackY *float64 `json:"black_y,omitempty"`
	WhiteY *float64 `json:"white_y,omitempty"`
	// Gamma, when present, shapes the GC16 mid-level ramp (1.0 = perceptually
	// linear, >1 darkens mids); passed downstream as --gray-gamma.
	Gamma *float64 `json:"gamma,omitempty"`
}

// IsGrayscale reports whether this palette is a grayscale (GC16) palette --
// either an explicit gray ramp or the two luminance endpoints.
func (p *Palette) IsGrayscale() bool {
	return p != nil && (len(p.Grays) > 0 || p.BlackY != nil || p.WhiteY != nil)
}

// FetchConfig returns the full device config as a raw JSON string.
func (c *Client) FetchConfig() (string, error) {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/config", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Host = c.host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	return string(body), nil
}

// FetchProcessingSettings returns the device processing settings as a raw JSON string.
func (c *Client) FetchProcessingSettings() (string, error) {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/settings/processing", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Host = c.host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read processing settings: %w", err)
	}

	return string(body), nil
}

// FetchPalette returns the device color palette as a raw JSON string.
func (c *Client) FetchPalette() (string, error) {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/settings/palette", ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Host = c.host

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read palette: %w", err)
	}

	return string(body), nil
}

func (c *Client) PushConfig(config map[string]interface{}) error {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/config", ip)

	jsonData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Host = c.host
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("device returned status: %d", resp.StatusCode)
	}

	return nil
}
