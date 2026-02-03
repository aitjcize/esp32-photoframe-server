package service

import (
	"errors"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	db        *gorm.DB
	jwtSecret []byte
}

type JWTClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	KeyID    uint   `json:"key_id,omitempty"`
	jwt.RegisteredClaims
}

func NewAuthService(db *gorm.DB, secret string) *AuthService {
	// If no secret provided, generate or use default (in prod, MUST be provided)
	if secret == "" {
		secret = "default-insecure-secret-change-me"
	}
	return &AuthService{
		db:        db,
		jwtSecret: []byte(secret),
	}
}

// UserCount returns the number of registered users.
func (s *AuthService) UserCount() (int64, error) {
	var count int64
	err := s.db.Model(&model.User{}).Count(&count).Error
	return count, err
}

func (s *AuthService) Register(username, password string) error {
	// Check if user exists
	var count int64
	s.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return errors.New("username already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := model.User{
		Username: username,
		Password: string(hashedPassword),
	}

	return s.db.Create(&user).Error
}

func (s *AuthService) Login(username, password string) (string, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return "", errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", errors.New("invalid credentials")
	}

	return s.generateToken(&user)
}

func (s *AuthService) generateToken(user *model.User) (string, error) {
	claims := JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24 hour token
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) GenerateDeviceToken(userID uint, username string, name string) (string, error) {
	// Create API Key record
	apiKey := model.APIKey{
		UserID: userID,
		Name:   name,
	}
	if err := s.db.Create(&apiKey).Error; err != nil {
		return "", err
	}

	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		KeyID:    apiKey.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(87600 * time.Hour)), // 10 years
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "device",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) GetOrGenerateDeviceToken(userID uint, username string, name string) (string, error) {
	// Check for existing key with this name and revoke it to ensure 1:1 mapping and freshness
	var apiKey model.APIKey
	if err := s.db.Where("user_id = ? AND name = ?", userID, name).First(&apiKey).Error; err == nil {
		// Key exists. Delete it so we can create a fresh one
		s.db.Delete(&apiKey)
	}
	return s.GenerateDeviceToken(userID, username, name)
}

func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		// If KeyID is present, verify it exists in DB
		if claims.KeyID > 0 {
			var count int64
			s.db.Model(&model.APIKey{}).Where("id = ?", claims.KeyID).Count(&count)
			if count == 0 {
				return nil, errors.New("token revoked")
			}
		}
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func (s *AuthService) ListTokens(userID uint) ([]model.APIKey, error) {
	var tokens []model.APIKey
	err := s.db.Where("user_id = ?", userID).Find(&tokens).Error
	return tokens, err
}

func (s *AuthService) RevokeToken(userID uint, tokenID uint) error {
	return s.db.Where("user_id = ? AND id = ?", userID, tokenID).Delete(&model.APIKey{}).Error
}

func (s *AuthService) UpdateAccount(userID uint, oldPassword, newUsername, newPassword string) error {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return errors.New("user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return errors.New("invalid current password")
	}

	if newUsername != "" && newUsername != user.Username {
		var count int64
		s.db.Model(&model.User{}).Where("username = ?", newUsername).Count(&count)
		if count > 0 {
			return errors.New("username already taken")
		}
		user.Username = newUsername
	}

	if newPassword != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		user.Password = string(hashedPassword)
	}

	return s.db.Save(&user).Error
}
