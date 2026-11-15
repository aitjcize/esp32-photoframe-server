package telegram

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	tele "gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type SettingsProvider interface {
	Get(key string) (string, error)
}

type Pusher interface {
	PushToHost(device *model.Device, imagePath string, extraOpts map[string]string) error
}

type Bot struct {
	b        *tele.Bot
	db       *gorm.DB
	dataDir  string
	settings SettingsProvider
	pusher   Pusher
}

func NewBot(token string, db *gorm.DB, dataDir string, settings SettingsProvider, pusher Pusher) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		b:        b,
		db:       db,
		dataDir:  dataDir,
		settings: settings,
		pusher:   pusher,
	}
	bot.registerHandlers()

	return bot, nil
}

func (bot *Bot) Start() {
	log.Println("Telegram bot started")
	go bot.b.Start()
}

func (bot *Bot) Stop() {
	bot.b.Stop()
}

func (bot *Bot) registerHandlers() {
	bot.b.Handle("/start", func(c tele.Context) error {
		return c.Send("Hello! Send me a photo to add it to the gallery.")
	})

	bot.b.Handle(tele.OnPhoto, bot.handlePhoto)
}

func (bot *Bot) handlePhoto(c tele.Context) error {
	photo := c.Message().Photo

	galleryDir := filepath.Join(bot.dataDir, "photos", "gallery")
	if err := os.MkdirAll(galleryDir, 0755); err != nil {
		return c.Send("Failed to create gallery directory.")
	}

	destPath := filepath.Join(galleryDir, fmt.Sprintf("telegram_%d.jpg", time.Now().UnixNano()))

	if err := bot.b.Download(&photo.File, destPath); err != nil {
		return c.Send("Failed to download photo: " + err.Error())
	}

	width, height, orientation := decodeDimensions(destPath)

	img := model.Image{
		FilePath:    destPath,
		Source:      model.SourceGallery,
		UserID:      1,
		Status:      "pending",
		CreatedAt:   time.Now(),
		Caption:     c.Message().Caption,
		Width:       width,
		Height:      height,
		Orientation: orientation,
	}
	if err := bot.db.Create(&img).Error; err != nil {
		os.Remove(destPath)
		return c.Send("Failed to save photo: " + err.Error())
	}
	// File the photo under the gallery album chosen in Settings, if any. A
	// stale setting (album deleted) leaves the photo unfiled rather than
	// failing the upload.
	if albumStr, _ := bot.settings.Get("telegram_album_id"); albumStr != "" {
		if albumID, err := strconv.Atoi(albumStr); err == nil && albumID > 0 {
			var n int64
			bot.db.Model(&model.Album{}).Where("id = ? AND source = ?", albumID, model.SourceGallery).Count(&n)
			if n == 1 {
				if err := bot.db.Create(&model.ImageAlbumMembership{ImageID: img.ID, AlbumID: uint(albumID)}).Error; err != nil {
					log.Printf("telegram: file photo %d under album %d: %v", img.ID, albumID, err)
				}
			}
		}
	}

	pushEnabled, _ := bot.settings.Get("telegram_push_enabled")
	targetDeviceIDStr, _ := bot.settings.Get("telegram_target_device_id")

	if pushEnabled == "true" && targetDeviceIDStr != "" {
		statusMsg, err := bot.b.Send(c.Recipient(), "Connecting to devices...")
		if err != nil {
			log.Printf("Failed to send status message: %v", err)
			return err
		}

		targetIDs := strings.Split(targetDeviceIDStr, ",")
		var successDevices []string
		var failDevices []string

		for _, id := range targetIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}

			var device model.Device
			if err := bot.db.First(&device, id).Error; err != nil {
				log.Printf("Failed to find target device (ID: %s): %v", id, err)
				failDevices = append(failDevices, fmt.Sprintf("ID %s", id))
				continue
			}

			err = bot.pusher.PushToHost(&device, destPath, nil)
			if err != nil {
				log.Printf("Failed to push to device %s: %v", device.Name, err)
				failDevices = append(failDevices, device.Name)
			} else {
				successDevices = append(successDevices, device.Name)
			}
		}

		var summary strings.Builder
		summary.WriteString("Photo added to gallery!\n")

		if len(successDevices) > 0 {
			for _, name := range successDevices {
				summary.WriteString(fmt.Sprintf("✅ %s\n", name))
			}
		}

		if len(failDevices) > 0 {
			for _, name := range failDevices {
				summary.WriteString(fmt.Sprintf("❌ %s (Offline/Failed)\n", name))
			}
		}

		msg := summary.String()

		_, editErr := bot.b.Edit(statusMsg, msg)
		if editErr != nil {
			return c.Send(msg)
		}
		return nil
	}

	return c.Send("Photo added to gallery! It will show up next time the device awakes.")
}

func decodeDimensions(path string) (int, int, string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, "landscape"
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, "landscape"
	}
	orientation := "landscape"
	if cfg.Height > cfg.Width {
		orientation = "portrait"
	}
	return cfg.Width, cfg.Height, orientation
}
