package service

import (
	"errors"
	"fmt"
	"image"
	"log"
	"os"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/imageops"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"gorm.io/gorm"
)

type DeviceService struct {
	db        *gorm.DB
	settings  *SettingsService
	processor *ProcessorService
	overlay   *OverlayService
	pfClient  *photoframe.Client
}

func NewDeviceService(db *gorm.DB, settings *SettingsService, processor *ProcessorService, overlay *OverlayService, pfClient *photoframe.Client) *DeviceService {
	return &DeviceService{
		db:        db,
		settings:  settings,
		processor: processor,
		overlay:   overlay,
		pfClient:  pfClient,
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

func (s *DeviceService) AddDevice(host string, useDeviceParameter, enableCollage, showDate, showWeather bool, weatherLat, weatherLon float64) (*model.Device, error) {
	sysInfo, err := s.pfClient.FetchSystemInfo(host)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch system info: %w", err)
	}

	name := sysInfo.DeviceName
	width := sysInfo.Width
	height := sysInfo.Height

	// Fetch orientation
	config, err := s.pfClient.FetchDeviceConfig(host)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device config: %w", err)
	}
	orientation := config.DisplayOrientation

	// Fallback for name if still empty
	if name == "" {
		name = host
	}

	// Validate dimensions
	if width == 0 || height == 0 {
		return nil, errors.New("device dimensions are required")
	}

	if orientation == "" {
		orientation = "landscape"
	}

	device := &model.Device{
		Name:               name,
		Host:               host,
		Width:              width,
		Height:             height,
		Orientation:        orientation,
		UseDeviceParameter: useDeviceParameter,
		EnableCollage:      enableCollage,
		ShowDate:           showDate,
		ShowWeather:        showWeather,
		WeatherLat:         weatherLat,
		WeatherLon:         weatherLon,
	}
	if err := s.db.Create(device).Error; err != nil {
		return nil, err
	}
	return device, nil
}

func (s *DeviceService) UpdateDevice(id uint, name, host string, width, height int, orientation string, useDeviceParameter, enableCollage, showDate, showWeather bool, weatherLat, weatherLon float64) (*model.Device, error) {
	var device model.Device
	if err := s.db.First(&device, id).Error; err != nil {
		return nil, errors.New("device not found")
	}

	// Fetch dimensions if requested and changed to enabled (or if forcing a refresh, logic could be more complex but simple for now)
	// Signal to refresh: name is empty OR width/height is 0 OR orientation is empty
	shouldRefresh := name == "" || width == 0 || height == 0 || orientation == ""

	if shouldRefresh {
		// Fetch info
		sysInfo, err := s.pfClient.FetchSystemInfo(host)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch system info: %w", err)
		}
		if name == "" {
			name = sysInfo.DeviceName
		}
		width = sysInfo.Width
		height = sysInfo.Height

		// Fetch orientation
		config, err := s.pfClient.FetchDeviceConfig(host)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch device config: %w", err)
		}
		if config.DisplayOrientation != "" {
			orientation = config.DisplayOrientation
		}
	}

	if name == "" {
		name = device.Name // Keep existing if failed to fetch
	}
	if name == "" {
		name = host // Final fallback
	}
	// Validate dimensions
	if width == 0 || height == 0 {
		return nil, errors.New("device dimensions are required")
	}

	device.Name = name
	device.Host = host
	device.Width = width
	device.Height = height
	device.Orientation = orientation
	device.UseDeviceParameter = useDeviceParameter
	device.EnableCollage = enableCollage
	device.ShowDate = showDate
	device.ShowWeather = showWeather
	device.WeatherLat = weatherLat
	device.WeatherLon = weatherLon

	if err := s.db.Save(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *DeviceService) DeleteDevice(id uint) error {
	result := s.db.Delete(&model.Device{}, id)
	return result.Error
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

func (s *DeviceService) ConfigureDevice(deviceID uint, config map[string]interface{}) error {
	var device model.Device
	if err := s.db.First(&device, deviceID).Error; err != nil {
		return errors.New("device not found")
	}
	return s.pfClient.PushConfig(device.Host, config)
}

func (s *DeviceService) GetDeviceConfig(deviceID uint) (*photoframe.DeviceConfig, error) {
	var device model.Device
	if err := s.db.First(&device, deviceID).Error; err != nil {
		return nil, errors.New("device not found")
	}
	return s.pfClient.FetchDeviceConfig(device.Host)
}

// PushToHost processes an image file and pushes it to a target host
// This encapsulates the logic previously in Telegram bot
// Now includes fetching device parameters if configured
func (s *DeviceService) PushToHost(device *model.Device, imagePath string, extraOpts map[string]string) error {
	// 0. Fetch Device Parameters if enabled
	processingOpts := make(map[string]string)
	for k, v := range extraOpts {
		processingOpts[k] = v
	}

	if device.UseDeviceParameter {
		// 1. Fetch Dimensions
		sysInfo, err := s.pfClient.FetchSystemInfo(device.Host)
		if err == nil {
			device.Width = sysInfo.Width
			device.Height = sysInfo.Height
		} else {
			log.Printf("Failed to fetch dimensions for %s: %v", device.Name, err)
		}

		var procSettings *photoframe.ProcessingSettings
		var palette *photoframe.Palette

		// 2. Fetch Processing Settings and Palette
		procSettings, err = s.pfClient.FetchProcessingSettings(device.Host)
		if err != nil {
			log.Printf("Failed to fetch processing settings from %s: %v", device.Host, err)
		}

		palette, err = s.pfClient.FetchPalette(device.Host)
		if err != nil {
			log.Printf("Failed to fetch palette from %s: %v", device.Host, err)
		}

		fetchedOpts := s.processor.MapProcessingSettings(procSettings, palette)
		for k, v := range fetchedOpts {
			processingOpts[k] = v
		}
		log.Printf("Fetched processing parameters for %s", device.Name)
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

	// 4. Orientation-aware Smart Resize
	isTargetPortrait := logicalH > logicalW
	if device.Orientation == "portrait" {
		isTargetPortrait = true
	} else if device.Orientation == "landscape" {
		isTargetPortrait = false
	}

	if isTargetPortrait && logicalW > logicalH {
		logicalW, logicalH = logicalH, logicalW
	} else if !isTargetPortrait && logicalH > logicalW {
		logicalW, logicalH = logicalH, logicalW
	}

	resizedImg := imageops.SmartResizeToFill(srcImg, logicalW, logicalH)

	// 5. Apply Overlay or Cover
	var finalImg image.Image = resizedImg
	if s.overlay != nil {
		overlayOpts := OverlayOptions{
			ShowDate:    device.ShowDate,
			ShowWeather: device.ShowWeather,
			WeatherLat:  device.WeatherLat,
			WeatherLon:  device.WeatherLon,
		}
		imgWithOverlay, err := s.overlay.ApplyOverlay(resizedImg, overlayOpts)
		if err == nil {
			finalImg = imgWithOverlay
		} else {
			log.Printf("Failed to apply overlay: %v", err)
		}
	}

	// 6. Process for E-Paper
	// Pass NATIVE dimensions to CLI.
	// The CLI will detect Source (logicalW/H) vs Target (nativeW/H) orientation mismatch and rotate if needed.
	opts := map[string]string{
		"dimension": fmt.Sprintf("%dx%d", nativeW, nativeH),
	}

	// Merge extra options (device params)
	for k, v := range processingOpts {
		opts[k] = v
	}

	processedData, thumbData, err := s.processor.ProcessImage(finalImg, opts)
	if err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}

	if err := s.pfClient.PushImage(device.Host, processedData, thumbData); err != nil {
		return fmt.Errorf("failed to push to device: %w", err)
	}

	return nil
}
