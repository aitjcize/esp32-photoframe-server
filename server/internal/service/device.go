package service

import (
	"errors"
	"fmt"
	"image"
	"log"
	"os"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/pkg/imageops"
	"github.com/aitjcize/photoframe-server/server/pkg/photoframe"
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

func (s *DeviceService) AddDevice(name, host string) (*model.Device, error) {
	device := model.Device{
		Name: name,
		Host: host,
	}
	if err := s.db.Create(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *DeviceService) DeleteDevice(id uint) error {
	return s.db.Delete(&model.Device{}, id).Error
}

// --- Push Logic ---

// PushToDevice resolves a device ID to a host and pushes the image
func (s *DeviceService) PushToDevice(deviceID uint, imagePath string) error {
	var device model.Device
	if err := s.db.First(&device, deviceID).Error; err != nil {
		return errors.New("device not found")
	}
	return s.PushToHost(device.Host, imagePath)
}

// PushToHost processes an image file and pushes it to a target host
// This encapsulates the logic previously in Telegram bot
func (s *DeviceService) PushToHost(host, imagePath string) error {
	// 1. Open and Decode logic
	f, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open image: %w", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// 2. Resize/Crop to Target Dimensions
	orientation, _ := s.settings.Get("orientation")
	targetW, targetH := 800, 480
	if orientation == "portrait" {
		targetW, targetH = 480, 800
	}

	img = imageops.ResizeToFill(img, targetW, targetH)

	// 3. Apply Overlay
	if s.overlay != nil {
		imgWithOverlay, err := s.overlay.ApplyOverlay(img)
		if err != nil {
			log.Printf("Failed to apply overlay: %v", err)
			// Continue with original resized image
		} else {
			img = imgWithOverlay
		}
	}

	// 4. Process (Dither/Convert)
	pngBytes, thumbBytes, err := s.processor.ProcessImage(img, nil)
	if err != nil {
		return fmt.Errorf("failed to process image: %w", err)
	}

	// 5. Push via Client
	log.Printf("Pushing image to host: %s", host)
	if err := s.pfClient.PushImage(host, pngBytes, thumbBytes); err != nil {
		return fmt.Errorf("failed to push to device: %w", err)
	}

	return nil
}
