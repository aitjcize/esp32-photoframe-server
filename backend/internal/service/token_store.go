package service

import (
	"errors"
	"log"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// DBTokenStore holds the single Google Calendar OAuth token in the
// google_calendar_auths table. (Google Photos tokens live on their connection
// rows; see connectionTokenStore.)
type DBTokenStore struct {
	db *gorm.DB
}

// NewDBTokenStore wraps db.
func NewDBTokenStore(db *gorm.DB) *DBTokenStore {
	return &DBTokenStore{db: db}
}

func (s *DBTokenStore) GetToken() (*oauth2.Token, error) {
	var auth model.GoogleCalendarAuth
	result := s.db.First(&auth)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errors.New("no token found")
		}
		return nil, result.Error
	}

	token := &oauth2.Token{
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		Expiry:       auth.Expiry,
		TokenType:    "Bearer",
	}
	log.Printf("GetToken[calendar]: Retrieved AccessToken (len=%d), RefreshToken (len=%d), Expiry=%v", len(token.AccessToken), len(token.RefreshToken), token.Expiry)
	return token, nil
}

func (s *DBTokenStore) SaveToken(token *oauth2.Token) error {
	var existingAuth model.GoogleCalendarAuth
	s.db.First(&existingAuth) // Ignore error, it might not exist

	refreshToken := token.RefreshToken
	log.Printf("SaveToken[calendar]: Received AccessToken (len=%d), RefreshToken (len=%d), Expiry=%v", len(token.AccessToken), len(token.RefreshToken), token.Expiry)

	if refreshToken == "" {
		log.Println("SaveToken[calendar]: New refresh token is empty, using existing one")
		refreshToken = existingAuth.RefreshToken
	}

	auth := model.GoogleCalendarAuth{
		ID:           1, // Singleton record
		AccessToken:  token.AccessToken,
		RefreshToken: refreshToken,
		Expiry:       token.Expiry,
	}
	// Upsert
	return s.db.Save(&auth).Error
}

func (s *DBTokenStore) ClearToken() error {
	return s.db.Delete(&model.GoogleCalendarAuth{}, 1).Error
}
