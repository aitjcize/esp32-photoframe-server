package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
	"gorm.io/gorm"
)

type DeviceServiceDeps struct {
	DB             *gorm.DB
	Settings       *SettingsService
	Processor      *ProcessorService
	Renderer       *RendererService
	Weather        *weather.Client
	Calendar       *gcalendar.Client
	CalendarGoogle *googlephotos.Client
}

type DeviceService struct {
	db             *gorm.DB
	settings       *SettingsService
	processor      *ProcessorService
	renderer       *RendererService
	weather        *weather.Client
	calendar       *gcalendar.Client
	calendarGoogle *googlephotos.Client
}

func NewDeviceService(deps DeviceServiceDeps) *DeviceService {
	return &DeviceService{
		db:             deps.DB,
		settings:       deps.Settings,
		processor:      deps.Processor,
		renderer:       deps.Renderer,
		weather:        deps.Weather,
		calendar:       deps.Calendar,
		calendarGoogle: deps.CalendarGoogle,
	}
}

// --- CRUD Operations ---

func (s *DeviceService) ListDevices() ([]model.Device, error) {
	var devices []model.Device
	if err := s.db.Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

// SetHTTPPassword stores the password the frame requires on its own HTTP
// API, or forgets it with "", after checking it against the frame: a GET
// /api/config with the new password (with no credential at all for ""), so
// the check never spends one of the frame's guesses on the stored one. Kept
// off UpdateDevice deliberately: that signature is already long, and a
// write-only secret does not belong in a payload the UI round-trips.
//
// A password the frame rejects is not stored, and the error is a
// *FramePasswordError of kind FrameWrongPassword: stored, it would only
// spend the frame's guesses. When the frame cannot confirm the password --
// asleep or unreachable, refusing checks for now (429), open, or on firmware
// without passwords -- it is stored anyway, since the user may be entering
// it ahead of the frame's next wake, and unverified says why it could not be
// confirmed. unverified is nil when the frame confirmed it.
//
// Storing resets auth_required: the flag was about the password this one
// replaces. The exception is forgetting the password of a frame that still
// asks for one, which leaves the server locked out of it -- so the flag is
// set, which is the state the server is then in.
func (s *DeviceService) SetHTTPPassword(id uint, password string) (unverified *FramePasswordError, err error) {
	if err := photoframe.ValidateHTTPPassword(password); err != nil {
		return nil, err
	}

	// Exclusive, like a change on the frame: requests in flight with the old
	// password finish, and record what the frame made of it, before the new
	// one is stored -- else a 401 to the old one could land after the store
	// and flag the new one. The check below goes to the frame directly, not
	// through FrameClient, which would wait on this very lock.
	l := frameCredLock(id)
	l.Lock()
	defer l.Unlock()

	var device model.Device
	if err := s.db.Select("id", "host").First(&device, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}

	authRequired := false
	if device.Host == "" {
		unverified = &FramePasswordError{Kind: FrameNoHost}
	} else if check := checkFramePassword(device.Host, password); check != nil {
		if check.Kind == FrameWrongPassword {
			if password != "" {
				return nil, check
			}
			// Forgotten as asked, but the frame still wants one.
			check.NoneStored = true
			authRequired = true
		}
		unverified = check
	}

	updates := map[string]interface{}{"http_password": password}
	if !authRequired {
		updates["auth_required"] = false
		updates["auth_failed_at"] = nil
	}
	res := s.db.Model(&model.Device{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrDeviceNotFound
	}
	if authRequired {
		// The check that was refused went out just now, under this hold, so
		// nothing sent before it is still out to clear the flag it sets.
		now := time.Now()
		recordFrameAuth(s.db, id, photoframe.Answer{Status: http.StatusUnauthorized, SentAt: now, ReceivedAt: now})
	}
	return unverified, nil
}

// ErrDeviceNotFound is returned when an id names no device.
var ErrDeviceNotFound = errors.New("device not found")

func (s *DeviceService) AddDevice(host, httpPassword string, enableCollage, showDate, showPhotoDate, showWeather bool, weatherLat, weatherLon float64, layout string, displayMode string, showCalendar bool, calendarID string, dateFormat string) (*model.Device, error) {
	// Try to fetch device info (works on LAN, fails for remote devices)
	var name string
	var width, height int
	var orientation, boardName, displayType string

	var deviceConfig, deviceProc, devicePalette string
	var authRequired bool
	var authFailedAt *time.Time

	// A frame with its own HTTP API password must be probed with it, or every
	// fetch below 401s and the device lands with placeholder defaults.
	pfClient := photoframe.NewClientWithPassword(host, httpPassword)
	sysInfo, err := pfClient.FetchSystemInfo()
	if err != nil {
		if frameStatus(err) == http.StatusUnauthorized {
			// The frame wants a password this server does not have, or not
			// the one given. Noted on the device, so the webapp asks for it
			// right away instead of the device just looking unreachable.
			log.Printf("Device at %s refused the password it was added with (%v); adding it without its settings", host, err)
			authRequired = true
			now := time.Now()
			authFailedAt = &now
		} else {
			log.Printf("Could not reach device at %s (may be remote): %v", host, err)
		}
		// Use defaults for unreachable devices; dimensions will be updated on first image request
		name = host
		width = 800
		height = 480
		orientation = "landscape"
	} else {
		name = sysInfo.DeviceName
		width = sysInfo.Width
		height = sysInfo.Height
		boardName = sysInfo.BoardName
		displayType = sysInfo.DisplayType

		configRaw, cfgErr := pfClient.FetchConfig()
		if cfgErr == nil {
			deviceConfig = configRaw
			var parsed struct {
				DisplayOrientation string `json:"display_orientation"`
			}
			if json.Unmarshal([]byte(configRaw), &parsed) == nil && parsed.DisplayOrientation != "" {
				orientation = parsed.DisplayOrientation
			}
		}

		if procRaw, err := pfClient.FetchProcessingSettings(); err == nil {
			deviceProc = procRaw
		}
		if paletteRaw, err := pfClient.FetchPalette(); err == nil {
			devicePalette = paletteRaw
		}
	}

	if name == "" {
		name = host
	}
	if width == 0 || height == 0 {
		width = 800
		height = 480
	}
	if orientation == "" {
		orientation = "landscape"
	}

	if displayMode == "" {
		displayMode = "cover"
	}

	device := &model.Device{
		HTTPPassword:             httpPassword,
		AuthRequired:             authRequired,
		AuthFailedAt:             authFailedAt,
		Name:                     name,
		Host:                     host,
		Width:                    width,
		Height:                   height,
		Orientation:              orientation,
		BoardName:                boardName,
		DisplayType:              displayType,
		EnableCollage:            enableCollage,
		ShowDate:                 showDate,
		ShowPhotoDate:            showPhotoDate,
		ShowWeather:              showWeather,
		WeatherLat:               weatherLat,
		WeatherLon:               weatherLon,
		Layout:                   layout,
		DisplayMode:              displayMode,
		ShowCalendar:             showCalendar,
		CalendarID:               calendarID,
		DateFormat:               dateFormat,
		DeviceConfig:             deviceConfig,
		DeviceProcessingSettings: deviceProc,
		DeviceColorPalette:       devicePalette,
	}
	if err := s.db.Create(device).Error; err != nil {
		return nil, err
	}
	// AfterFind has not run on a freshly created row.
	device.HTTPPasswordSet = device.HTTPPassword != ""
	return device, nil
}

// UpdateDevice writes only fields the server owns or shares with the device
// (Name, Host, Orientation, and the render/overlay settings). It never
// contacts the device, so offline edits succeed — shared fields (Name,
// Orientation) propagate to the device via the separate updateDeviceConfig
// path (push-if-online, else X-Config-Payload on next fetch).
//
// Hardware-derived fields (Width, Height, BoardName, DeviceConfig,
// DeviceProcessingSettings, DeviceColorPalette) are only written by
// AddDevice and RefreshDeviceFromHardware.
func (s *DeviceService) UpdateDevice(id uint, name, host, orientation string, enableCollage, showDate, showPhotoDate, showWeather bool, weatherLat, weatherLon float64, aiProvider, aiModel, aiPrompt string, layout string, displayMode string, showCalendar bool, calendarID string, dateFormat string) (*model.Device, error) {
	// It may change the host, and with it reset the frame's refusal (below).
	// That needs the lock held exclusively, like a password change, so a
	// request already on its way to the old host finishes, and records that
	// frame's answer, before the new host is stored -- else a late 401 from
	// the old frame would land on the new one. Any other edit takes it
	// shared, as before: a new name must not wait two minutes behind a push
	// to a frame that has stopped answering. The host is peeked at under the
	// shared lock to choose, and checked again under the exclusive one.
	l := frameCredLock(id)
	exclusive := false
	var device model.Device
	for {
		if exclusive {
			l.Lock()
		} else {
			l.RLock()
		}
		if err := s.db.First(&device, id).Error; err != nil {
			if exclusive {
				l.Unlock()
			} else {
				l.RUnlock()
			}
			return nil, errors.New("device not found")
		}
		if exclusive || host == device.Host {
			break
		}
		l.RUnlock()
		exclusive = true
	}
	defer func() {
		if exclusive {
			l.Unlock()
		} else {
			l.RUnlock()
		}
	}()

	if name == "" {
		name = device.Name // Keep existing if blank
	}
	if name == "" {
		name = host // Final fallback
	}
	if orientation == "" {
		orientation = device.Orientation
	}
	if displayMode == "" {
		displayMode = "cover"
	}

	hostChanged := host != device.Host
	device.Name = name
	device.Host = host
	device.Orientation = orientation
	device.EnableCollage = enableCollage
	device.ShowDate = showDate
	device.ShowPhotoDate = showPhotoDate
	device.ShowWeather = showWeather
	device.WeatherLat = weatherLat
	device.WeatherLon = weatherLon
	device.AIProvider = aiProvider
	device.AIModel = aiModel
	device.AIPrompt = aiPrompt
	device.Layout = layout
	device.DisplayMode = displayMode
	device.ShowCalendar = showCalendar
	device.CalendarID = calendarID
	device.DateFormat = dateFormat

	// The password is only written by SetHTTPPassword and ChangeFramePassword,
	// and the auth flag by the frame's answers as they come in; saving the
	// copies loaded above could undo a change made meanwhile. A new host is
	// the exception for the flag: the refusal was the old host's frame's,
	// and the frame at the new one has not answered anything yet.
	omit := []string{"http_password"}
	if hostChanged {
		device.AuthRequired = false
		device.AuthFailedAt = nil
	} else {
		omit = append(omit, "auth_required", "auth_failed_at")
	}
	if err := s.db.Omit(omit...).Save(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// RefreshDeviceFromHardware pulls live state from the device (dimensions,
// board name, config, processing settings, palette) and writes it onto the
// stored row. Unlike UpdateDevice this requires the device to be reachable
// and returns an error if any of the critical fetches fail.
func (s *DeviceService) RefreshDeviceFromHardware(id uint) (*model.Device, error) {
	var device model.Device
	if err := s.db.First(&device, id).Error; err != nil {
		return nil, errors.New("device not found")
	}

	// Held to the end, Save included: a frame password change waits for it.
	// At the loaded host: the data fetched is saved onto that row.
	pfClient, release, err := FrameClientAt(s.db, id, device.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from device: %w", err)
	}
	defer release()

	sysInfo, err := pfClient.FetchSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch system info: %w", err)
	}
	if sysInfo.DeviceName != "" {
		device.Name = sysInfo.DeviceName
	}
	device.Width = sysInfo.Width
	device.Height = sysInfo.Height
	if sysInfo.BoardName != "" {
		device.BoardName = sysInfo.BoardName
	}
	if sysInfo.DisplayType != "" {
		device.DisplayType = sysInfo.DisplayType
	}

	configRaw, err := pfClient.FetchConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device config: %w", err)
	}
	device.DeviceConfig = configRaw
	var parsedConfig struct {
		DisplayOrientation string `json:"display_orientation"`
	}
	if json.Unmarshal([]byte(configRaw), &parsedConfig) == nil && parsedConfig.DisplayOrientation != "" {
		device.Orientation = parsedConfig.DisplayOrientation
	}

	if procRaw, err := pfClient.FetchProcessingSettings(); err != nil {
		log.Printf("Failed to fetch processing settings from %s: %v", device.Host, err)
	} else {
		device.DeviceProcessingSettings = procRaw
	}

	if paletteRaw, err := pfClient.FetchPalette(); err != nil {
		log.Printf("Failed to fetch palette from %s: %v", device.Host, err)
	} else {
		device.DeviceColorPalette = paletteRaw
	}

	// Refresh writes neither the password nor the host, nor the auth flag:
	// saving the copies loaded above could undo an edit made meanwhile -- or,
	// for the flag, what the frame's answers to these very fetches recorded.
	if err := s.db.Omit("http_password", "host", "auth_required", "auth_failed_at").Save(&device).Error; err != nil {
		return nil, err
	}

	// The copy returned reports the flag as those answers left it on the row
	// -- cleared by the fetches that went through, set again should the
	// frame have refused one of the optional ones -- rather than the state
	// loaded before them, or one assumed from the required fetches alone.
	// Still under the hold above, so nothing else has changed it since.
	var recorded model.Device
	if err := s.db.Select("auth_required", "auth_failed_at").First(&recorded, id).Error; err != nil {
		return nil, err
	}
	device.AuthRequired = recorded.AuthRequired
	device.AuthFailedAt = recorded.AuthFailedAt
	return &device, nil
}

func (s *DeviceService) DeleteDevice(id uint) error {
	// Not in the middle of a frame password change, whose result would
	// then have nowhere to be stored.
	defer holdFrameCreds(id)()
	// device_histories, device_album_mappings, device_url_mappings and
	// generative_states are removed automatically via ON DELETE CASCADE
	// (migration 000032, enforced by _foreign_keys=on) — Device has no
	// soft-delete, so this is a real DELETE and the cascade fires. Only api_keys
	// needs explicit handling: its device_id has no FK and tokens are unbound,
	// not deleted, so a token survives the device it pointed at.
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.APIKey{}).
			Where("device_id = ?", id).Update("device_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Device{}, id).Error
	})
}

// --- Push Logic ---

// PushToDevice resolves a device ID to a host and pushes the image
func (s *DeviceService) PushToDevice(deviceID uint, imagePath string) error {
	var device model.Device
	if err := s.db.First(&device, deviceID).Error; err != nil {
		return errors.New("device not found")
	}

	if err := s.PushToHost(&device, imagePath, nil); err != nil {
		return err
	}

	return nil
}

// PushToHost processes an image file and pushes it to a target host
// This encapsulates the logic previously in Telegram bot
// Now includes fetching device parameters if configured
func (s *DeviceService) PushToHost(device *model.Device, imagePath string, extraOpts map[string]string) error {
	// 0. Fetch system info to determine firmware version and optionally device parameters
	processingOpts := make(map[string]string)
	for k, v := range extraOpts {
		processingOpts[k] = v
	}

	// Always fetch system info for firmware version check
	pfClient, release, sysInfoErr := FrameClientAt(s.db, device.ID, device.Host)
	var sysInfo *photoframe.SystemInfo
	if sysInfoErr == nil {
		sysInfo, sysInfoErr = pfClient.FetchSystemInfo()
		release()
	}
	if sysInfoErr != nil {
		// A refused password (401) stays refused and a locked-out frame
		// (429) refuses everything: the push would fail the same way after
		// all the rendering, and a 401 would spend another of the frame's
		// guesses.
		if IsFrameAuthError(sysInfoErr) {
			return fmt.Errorf("failed to push to device: %w", sysInfoErr)
		}
		log.Printf("Failed to fetch system info for %s: %v", device.Name, sysInfoErr)
	}

	// Use PNG for older firmware that doesn't support epdgz
	if sysInfoErr != nil || !photoframe.SupportsEPDGZ(sysInfo.Version) {
		processingOpts["format"] = "png"
	}

	// 1. Validate dimensions
	nativeW, nativeH := device.Width, device.Height
	if nativeW == 0 || nativeH == 0 {
		nativeW, nativeH = 800, 480
	}
	logicalW, logicalH := nativeW, nativeH

	// 2. Open file
	f, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	defer f.Close()

	// 3. Decode
	srcImg, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// 4. Apply device orientation to logical dimensions (for overlay rendering)
	orientation := device.Orientation
	if orientation == "portrait" && logicalW > logicalH {
		logicalW, logicalH = logicalH, logicalW
	} else if orientation == "landscape" && logicalW < logicalH {
		logicalW, logicalH = logicalH, logicalW
	}

	// Parse the device's stored processing settings once; nil if absent or
	// empty. The synced scaleMode decides the overlay layout below, and
	// MapProcessingSettings later maps the rest onto converter options (it
	// still emits the grayscale palette for nil settings and leaves
	// tone/dither to the CLI's defaults, so no zero-valued fallback is
	// needed -- that would have forced exposure/contrast to 0).
	var pushSettings *photoframe.ProcessingSettings
	if raw := strings.TrimSpace(device.DeviceProcessingSettings); raw != "" && raw != "{}" {
		var ps photoframe.ProcessingSettings
		if err := json.Unmarshal([]byte(raw), &ps); err == nil {
			pushSettings = &ps
		} else {
			log.Printf("Failed to parse stored processing settings for %s: %v", device.Name, err)
		}
	}

	// 5. Render layout (photo + overlay + calendar)
	needsOverlay := device.ShowDate || device.ShowPhotoDate || device.ShowWeather || device.ShowCalendar
	var finalImg image.Image

	if needsOverlay {
		var weatherData *weather.CurrentWeather
		var deviceTimezone string
		if device.ShowWeather && device.WeatherLat != 0 && device.WeatherLon != 0 {
			latStr := fmt.Sprintf("%f", device.WeatherLat)
			lonStr := fmt.Sprintf("%f", device.WeatherLon)
			var weatherErr error
			weatherData, weatherErr = s.weather.GetWeather(latStr, lonStr)
			if weatherErr != nil {
				log.Printf("Failed to fetch weather data for device %d: %v", device.ID, weatherErr)
			}
			if weatherData != nil {
				deviceTimezone = weatherData.Timezone
			}
		}

		var events []gcalendar.Event
		if device.ShowCalendar && s.calendar != nil && s.calendarGoogle != nil {
			httpClient, err := s.calendarGoogle.GetClient()
			if err == nil {
				calendarID := device.CalendarID
				if calendarID == "" {
					calendarID = "primary"
				}
				var calErr error
				events, calErr = s.calendar.GetTodayEvents(httpClient, calendarID, deviceTimezone)
				if calErr != nil {
					log.Printf("Failed to fetch calendar events for device %d: %v", device.ID, calErr)
				}
			}
		}

		layout := device.Layout
		if layout == "" {
			layout = model.LayoutPhotoOverlay
		}
		displayMode := device.DisplayMode
		if pushSettings != nil && pushSettings.ScaleMode != "" {
			// The device-synced scale mode wins over the legacy column
			displayMode = pushSettings.ScaleMode
		}
		if displayMode == "" {
			displayMode = "cover"
		}

		var renderErr error
		finalImg, renderErr = s.renderer.Render(RenderOptions{
			Layout:        layout,
			DisplayMode:   displayMode,
			Width:         logicalW,
			Height:        logicalH,
			NativeWidth:   nativeW,
			NativeHeight:  nativeH,
			Photo:         srcImg,
			ShowDate:      device.ShowDate,
			ShowPhotoDate: device.ShowPhotoDate,
			ShowWeather:   device.ShowWeather,
			Weather:       weatherData,
			ShowCalendar:  device.ShowCalendar,
			Events:        events,
			Timezone:      deviceTimezone,
			DateFormat:    device.DateFormat,
		})
		if renderErr != nil {
			return fmt.Errorf("render failed: %w", renderErr)
		}
	} else {
		finalImg = srcImg
	}

	// 6. Process for E-Paper
	// Always pass native panel dimensions. The CLI handles orientation
	// internally (swaps dims, processes, rotates output to native layout).
	opts := map[string]string{
		"dimension": fmt.Sprintf("%dx%d", nativeW, nativeH),
	}
	if orientation != "" {
		opts["orientation"] = orientation
	}

	// Merge processing settings + palette so the CLI quantizes to the right
	// color model. Without this a grayscale (GC16) panel would fall back to
	// Spectra-6 color quantization. Mirrors what the /image handler does.
	grayscale := device.IsGrayscale()

	// Parse the device's stored color palette (JSON string column); nil if
	// absent or empty ("{}"). For grayscale panels this supplies a calibrated
	// gray ramp via Palette.Grays.
	var palette *photoframe.Palette
	if raw := strings.TrimSpace(device.DeviceColorPalette); raw != "" && raw != "{}" {
		var p photoframe.Palette
		if err := json.Unmarshal([]byte(raw), &p); err == nil {
			palette = &p
		} else {
			log.Printf("Failed to parse stored palette for %s: %v", device.Name, err)
		}
	}

	// (The stored processing settings were parsed above, before the render,
	// so the overlay layout and the converter agree on the scale mode.)
	settings := pushSettings

	// Merge the mapped CLI options (palette, tone-mapping, etc.) without
	// clobbering dimension/orientation/format already set above.
	mapped := s.processor.MapProcessingSettings(settings, palette, grayscale)
	for k, v := range mapped {
		if k == "dimension" || k == "orientation" || k == "format" {
			continue
		}
		opts[k] = v
	}

	// Merge extra options (device params) last so explicit overrides win.
	for k, v := range processingOpts {
		opts[k] = v
	}

	processedData, thumbData, err := s.processor.ProcessImage(finalImg, opts)
	if err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}

	// A fresh client, not the one above: the frame's password may have been
	// changed while the image was being rendered. Same host, though: the
	// image was rendered for the frame there.
	pfClient, release, err = FrameClientAt(s.db, device.ID, device.Host)
	if err != nil {
		return fmt.Errorf("failed to push to device: %w", err)
	}
	defer release()
	if err := pfClient.PushImage(processedData, thumbData); err != nil {
		return fmt.Errorf("failed to push to device: %w", err)
	}

	return nil
}
