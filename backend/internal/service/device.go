package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"os"
	"strings"

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

func (s *DeviceService) AddDevice(host string, enableCollage, showDate, showPhotoDate, showWeather bool, weatherLat, weatherLon float64, layout string, displayMode string, showCalendar bool, calendarID string, dateFormat string) (*model.Device, error) {
	// Try to fetch device info (works on LAN, fails for remote devices)
	var name string
	var width, height int
	var orientation, boardName, displayType string

	var deviceConfig, deviceProc, devicePalette string

	pfClient := photoframe.NewClient(host)
	sysInfo, err := pfClient.FetchSystemInfo()
	if err != nil {
		log.Printf("Could not reach device at %s (may be remote): %v", host, err)
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
	var device model.Device
	if err := s.db.First(&device, id).Error; err != nil {
		return nil, errors.New("device not found")
	}

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

	if err := s.db.Save(&device).Error; err != nil {
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

	pfClient := photoframe.NewClient(device.Host)

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

	if err := s.db.Save(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *DeviceService) DeleteDevice(id uint) error {
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
	pfClient := photoframe.NewClient(device.Host)
	sysInfo, sysInfoErr := pfClient.FetchSystemInfo()
	if sysInfoErr != nil {
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

	if err := pfClient.PushImage(processedData, thumbData); err != nil {
		return fmt.Errorf("failed to push to device: %w", err)
	}

	return nil
}
