package telegram

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	tele "gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type SettingsProvider interface {
	Get(key string) (string, error)
}

type Pusher interface {
	PushToHost(host, imagePath string) error
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
	deviceHost, _ := bot.settings.Get("device_host")

	if pushEnabled == "true" && deviceHost != "" {
		// Send initial status
		statusMsg, err := bot.b.Send(c.Recipient(), "Connecting to device...")
		if err != nil {
			log.Printf("Failed to send status message: %v", err)
			return err
		}

		err = bot.pusher.PushToHost(deviceHost, destPath)
		if err != nil {
			log.Printf("Failed to push to device: %v", err)
			_, editErr := bot.b.Edit(statusMsg, "Photo updated! Device is offline/unreachable, so it will show up next time the device awakes.")
			if editErr != nil {
				return c.Send("Photo updated! Device is offline/unreachable, so it will show up next time the device awakes.")
			}
			return nil
		}

		_, editErr := bot.b.Edit(statusMsg, "Photo updated and displayed on device!")
		if editErr != nil {
			return c.Send("Photo updated and displayed on device!")
		}
		return nil
	}

	return c.Send("Photo updated! It will show up next time the device awakes.")
}
