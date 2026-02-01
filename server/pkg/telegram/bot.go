package telegram

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
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
		return c.Send("Hello! Send me a photo to display on your frame.")
	})

	bot.b.Handle(tele.OnPhoto, bot.handlePhoto)
}

func (bot *Bot) handlePhoto(c tele.Context) error {
	// Download photo
	photo := c.Message().Photo

	// Create directory if not exists
	photosDir := filepath.Join(bot.dataDir, "photos")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		return c.Send("Failed to create photos directory.")
	}

	// Target file path
	destPath := filepath.Join(photosDir, "telegram_last.jpg")

	// Download
	if err := bot.b.Download(&photo.File, destPath); err != nil {
		return c.Send("Failed to download photo: " + err.Error())
	}

	// Update Caption Setting
	caption := c.Message().Caption
	var setting model.Setting
	setting.Key = "telegram_caption"
	setting.Value = caption
	bot.db.Save(&setting)

	// Check if Push to Device is enabled
	pushEnabled, _ := bot.settings.Get("telegram_push_enabled")
	targetDeviceIDStr, _ := bot.settings.Get("telegram_target_device_id")

	if pushEnabled == "true" && targetDeviceIDStr != "" {
		// Send initial status
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

			// Look up device
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
		summary.WriteString("Photo updated!\n")

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

	return c.Send("Photo updated! It will show up next time the device awakes.")
}
