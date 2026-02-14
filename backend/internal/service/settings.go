package service

import (
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"gorm.io/gorm"
)

// CalendarConfigProvider wraps SettingsService to provide calendar-specific OAuth config.
// It implements googlephotos.ConfigProvider so we can create a separate OAuth client for calendar.
type CalendarConfigProvider struct {
	settings *SettingsService
}

func NewCalendarConfigProvider(s *SettingsService) *CalendarConfigProvider {
	return &CalendarConfigProvider{settings: s}
}

func (p *CalendarConfigProvider) GetGoogleConfig() (googlephotos.Config, error) {
	return p.settings.GetGoogleCalendarConfig()
}

type SettingsService struct {
	db *gorm.DB
}

func NewSettingsService(db *gorm.DB) *SettingsService {
	return &SettingsService{db: db}
}

func (s *SettingsService) Get(key string) (string, error) {
	var setting model.Setting
	result := s.db.First(&setting, "key = ?", key)
	if result.Error != nil {
		return "", result.Error
	}
	return setting.Value, nil
}

func (s *SettingsService) Set(key string, value string) error {
	setting := model.Setting{Key: key, Value: value}
	// Save will create or update
	return s.db.Save(&setting).Error
}

func (s *SettingsService) GetAll() (map[string]string, error) {
	var settings []model.Setting
	result := s.db.Find(&settings)
	if result.Error != nil {
		return nil, result.Error
	}

	settingsMap := make(map[string]string)
	for _, setting := range settings {
		settingsMap[setting.Key] = setting.Value
	}
	return settingsMap, nil
}

func (s *SettingsService) GetGoogleConfig() (googlephotos.Config, error) {
	clientID, _ := s.Get("google_client_id")
	clientSecret, _ := s.Get("google_client_secret")

	return googlephotos.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "", // Will be set dynamically
		Scopes: []string{
			"https://www.googleapis.com/auth/photospicker.mediaitems.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
	}, nil
}

func (s *SettingsService) GetGoogleCalendarConfig() (googlephotos.Config, error) {
	clientID, _ := s.Get("google_client_id")
	clientSecret, _ := s.Get("google_client_secret")

	return googlephotos.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "", // Will be set dynamically
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
	}, nil
}
