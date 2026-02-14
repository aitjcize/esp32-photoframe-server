package service

import (
	"errors"
	"log"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type DBTokenStore struct {
	db   *gorm.DB
	kind string // "photos" or "calendar"
}

func NewDBTokenStore(db *gorm.DB, kind string) *DBTokenStore {
	return &DBTokenStore{db: db, kind: kind}
}

func (s *DBTokenStore) GetToken() (*oauth2.Token, error) {
	if s.kind == "calendar" {
		return s.getCalendarToken()
	}
	return s.getPhotosToken()
}

func (s *DBTokenStore) SaveToken(token *oauth2.Token) error {
	if s.kind == "calendar" {
		return s.saveCalendarToken(token)
	}
	return s.savePhotosToken(token)
}

func (s *DBTokenStore) ClearToken() error {
	if s.kind == "calendar" {
		return s.db.Delete(&model.GoogleCalendarAuth{}, 1).Error
	}
	return s.db.Delete(&model.GoogleAuth{}, 1).Error
}

// Photos token methods (existing logic)

func (s *DBTokenStore) getPhotosToken() (*oauth2.Token, error) {
	var auth model.GoogleAuth
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
	log.Printf("GetToken[photos]: Retrieved AccessToken (len=%d), RefreshToken (len=%d), Expiry=%v", len(token.AccessToken), len(token.RefreshToken), token.Expiry)
	return token, nil
}

func (s *DBTokenStore) savePhotosToken(token *oauth2.Token) error {
	var existingAuth model.GoogleAuth
	s.db.First(&existingAuth) // Ignore error, it might not exist

	refreshToken := token.RefreshToken
	log.Printf("SaveToken[photos]: Received AccessToken (len=%d), RefreshToken (len=%d), Expiry=%v", len(token.AccessToken), len(token.RefreshToken), token.Expiry)

	if refreshToken == "" {
		log.Println("SaveToken[photos]: New refresh token is empty, using existing one")
		refreshToken = existingAuth.RefreshToken
	}

	log.Printf("SaveToken[photos]: Saving RefreshToken (len=%d)", len(refreshToken))

	auth := model.GoogleAuth{
		ID:           1, // Singleton record
		AccessToken:  token.AccessToken,
		RefreshToken: refreshToken,
		Expiry:       token.Expiry,
	}
	// Upsert
	return s.db.Save(&auth).Error
}

// Calendar token methods

func (s *DBTokenStore) getCalendarToken() (*oauth2.Token, error) {
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

func (s *DBTokenStore) saveCalendarToken(token *oauth2.Token) error {
	var existingAuth model.GoogleCalendarAuth
	s.db.First(&existingAuth) // Ignore error, it might not exist

	refreshToken := token.RefreshToken
	log.Printf("SaveToken[calendar]: Received AccessToken (len=%d), RefreshToken (len=%d), Expiry=%v", len(token.AccessToken), len(token.RefreshToken), token.Expiry)

	if refreshToken == "" {
		log.Println("SaveToken[calendar]: New refresh token is empty, using existing one")
		refreshToken = existingAuth.RefreshToken
	}

	log.Printf("SaveToken[calendar]: Saving RefreshToken (len=%d)", len(refreshToken))

	auth := model.GoogleCalendarAuth{
		ID:           1, // Singleton record
		AccessToken:  token.AccessToken,
		RefreshToken: refreshToken,
		Expiry:       token.Expiry,
	}
	// Upsert
	return s.db.Save(&auth).Error
}
