package photoframe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// basicAuthTransport attaches the frame's HTTP password to every request.
// Doing it at the transport rather than at each call site means a newly added
// request cannot forget it -- there are seven of them in this file already.
type basicAuthTransport struct {
	base     http.RoundTripper
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone before mutating: RoundTrip must not modify the caller's request.
	r := req.Clone(req.Context())
	// The firmware ignores the username; the password is the whole credential.
	r.SetBasicAuth("photoframe", t.password)
	return t.base.RoundTrip(r)
}

// Answer is what a status observer sees of each request the frame answered:
// the status, and when the request went out and the answer came in. Two
// requests can be out at once, and an observer that orders their answers
// needs both times.
type Answer struct {
	Status     int
	SentAt     time.Time
	ReceivedAt time.Time
}

// statusObserverTransport reports every answer to fn. Like
// basicAuthTransport it sits at the transport, so every request the client
// makes is seen, including ones added later.
type statusObserverTransport struct {
	base http.RoundTripper
	fn   func(Answer)
}

func (t *statusObserverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	sentAt := time.Now()
	resp, err := t.base.RoundTrip(req)
	if err == nil {
		t.fn(Answer{Status: resp.StatusCode, SentAt: sentAt, ReceivedAt: time.Now()})
	}
	return resp, err
}

type Client struct {
	host       string
	resolvedIP string // Cached resolved IP
	httpClient *http.Client
}

// WithStatusObserver has the client call fn with every answer the frame
// gives, whatever the request. The device service uses it to keep a
// device's auth_required flag in step with the frame: a 401 sets it, an
// answer to the same password clears it. Doing it under every request means
// no call site can forget it. Returns c for chaining.
func (c *Client) WithStatusObserver(fn func(Answer)) *Client {
	base := c.httpClient
	c.httpClient = &http.Client{
		Transport:     &statusObserverTransport{base: base.Transport, fn: fn},
		Timeout:       base.Timeout,
		CheckRedirect: base.CheckRedirect,
	}
	return c
}

// WithTimeout bounds every request this client makes to d, in place of the
// shared client's two minutes, which are sized for an image upload. For a
// request made while others wait on it. Returns c for chaining.
func (c *Client) WithTimeout(d time.Duration) *Client {
	base := c.httpClient
	c.httpClient = &http.Client{
		Transport:     base.Transport,
		Timeout:       d,
		CheckRedirect: base.CheckRedirect,
	}
	return c
}

// MaxHTTPPasswordLen is the longest password the firmware stores
// (HTTP_PASSWORD_MAX_LEN - 1). It silently truncates anything longer, so a
// longer value here would never match.
const MaxHTTPPasswordLen = 63

// ValidateHTTPPassword reports why the firmware could not store password as
// given: too long (it would be refused), or holding a NUL byte (it would keep
// only what comes before it, so the full value would never match).
func ValidateHTTPPassword(password string) error {
	if len(password) > MaxHTTPPasswordLen {
		return fmt.Errorf("frame password must be at most %d bytes", MaxHTTPPasswordLen)
	}
	if strings.IndexByte(password, 0) >= 0 {
		return errors.New("frame password must not contain a NUL character")
	}
	return nil
}

func NewClient(host string) *Client {
	return NewClientWithPassword(host, "")
}

// NewClientWithPassword talks to a frame whose HTTP API is password-protected
// (esp32-photoframe #130). An empty password means the frame is open, which is
// the firmware default, and yields the plain shared client.
func NewClientWithPassword(host, password string) *Client {
	httpClient := sharedHTTPClient
	if password != "" {
		// Wrap the shared transport so connection pooling is still shared.
		httpClient = &http.Client{
			Transport: &basicAuthTransport{base: sharedHTTPClient.Transport, password: password},
			Timeout:   sharedHTTPClient.Timeout,
			// The transport attaches the password to every request it sees,
			// so following a redirect could hand it to another host. The
			// firmware never redirects its API, so just don't follow.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{
		host:       host,
		httpClient: httpClient,
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
		return newStatusError(resp)
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

	// A host with an explicit port ("192.168.1.10:8080") resolves its name
	// and keeps the port. A bare IPv6 literal fails to split and is used whole.
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		name, port = host, ""
	}
	ip, err := lookupIP(name)
	if err != nil {
		return "", err
	}
	if port != "" {
		ip = net.JoinHostPort(ip, port)
	}
	c.resolvedIP = ip
	return ip, nil
}

func lookupIP(host string) (string, error) {
	// If it's already an IP, return it
	if net.ParseIP(host) != nil {
		return host, nil
	}

	// For .local (mDNS) on macOS, use dns-sd for fast resolution
	// (Go's net.LookupHost has a 5s timeout trying regular DNS first)
	if strings.HasSuffix(host, ".local") && runtime.GOOS == "darwin" {
		if ip, err := resolveMDNSDarwin(host); err == nil {
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
			return ip, nil
		}
	}

	// Fallback to first (likely IPv6)
	if len(ips) > 0 {
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
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "80")
	}

	// "tcp", not "tcp4": resolveHost can return an IPv6 address.
	conn, err := net.DialTimeout("tcp", target, 2*time.Second)
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
		return nil, newStatusError(resp)
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
	ScaleMode            string  `json:"scaleMode"`
	BackgroundColor      string  `json:"backgroundColor"`
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
		return "", newStatusError(resp)
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
		return "", newStatusError(resp)
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
		return "", newStatusError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read palette: %w", err)
	}

	return string(body), nil
}

// PushProcessingSettings POSTs a full processing-settings object to the
// device. The firmware overlays the payload onto defaults, so callers must
// send the complete object, not a partial update.
func (c *Client) PushProcessingSettings(settings []byte) error {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/settings/processing", ip)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(settings))
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
		return newStatusError(resp)
	}

	return nil
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
		return newStatusError(resp)
	}

	return nil
}

// StatusError is a non-200 answer from the frame, returned by every request
// in this file. Error() keeps the "device returned status: N" wording;
// callers that need to tell a rejected password (401) or a lockout (429)
// apart use errors.As.
type StatusError struct {
	StatusCode int
	// RetryAfter is the frame's Retry-After header on a 429, in seconds.
	RetryAfter string
	// Message is the frame's own explanation, when its body carried one
	// ({"message": ...} from a rejected config, {"error": ...} from the
	// password gate).
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("device returned status: %d", e.StatusCode)
}

func newStatusError(resp *http.Response) *StatusError {
	e := &StatusError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	var body struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body) == nil {
		e.Message = body.Message
		if e.Message == "" {
			e.Message = body.Error
		}
	}
	return e
}

// PatchConfig sends a partial config update: fields left out keep their
// value on the frame. Unlike PushConfig's POST, which callers use with a full
// config, this is for changing one setting on its own -- in particular the
// frame's HTTP password, which the frame accepts only here, from a client
// that authenticated with the current one.
func (c *Client) PatchConfig(fields map[string]interface{}) error {
	ip, err := c.resolveHost(c.host)
	if err != nil {
		return fmt.Errorf("failed to resolve device %s: %w", c.host, err)
	}

	url := fmt.Sprintf("http://%s/api/config", ip)

	jsonData, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
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
		return newStatusError(resp)
	}

	return nil
}
