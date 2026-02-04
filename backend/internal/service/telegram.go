package service

import (
	"log"
	"sync"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/telegram"
	"gorm.io/gorm"
)

type TelegramService struct {
	bot      *telegram.Bot
	db       *gorm.DB
	dataDir  string
	settings *SettingsService
	pusher   telegram.Pusher
	mu       sync.Mutex
}

func NewTelegramService(db *gorm.DB, dataDir string, settings *SettingsService, pusher telegram.Pusher) *TelegramService {
	return &TelegramService{
		db:       db,
		dataDir:  dataDir,
		settings: settings,
		pusher:   pusher,
	}
}

func (s *TelegramService) Restart(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bot != nil {
		s.bot.Stop()
		s.bot = nil
	}

	if token == "" {
		log.Println("Telegram bot stopped (no token provided)")
		return
	}

	bot, err := telegram.NewBot(token, s.db, s.dataDir, s.settings, s.pusher)
	if err != nil {
		log.Printf("Failed to start Telegram bot: %v", err)
		return
	}

	s.bot = bot
	s.bot.Start()
	log.Println("Telegram bot started/restarted")
}
