package service

import (
	"errors"
	"log"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
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
	DeviceID uint   `json:"device_id,omitempty"`
	jwt.RegisteredClaims
}

func NewAuthService(db *gorm.DB, secret string) *AuthService {
	// If no secret provided, generate or use default (in prod, MUST be provided)
	if secret == "" {
		secret = "default-insecure-secret-change-me"
		log.Println("WARNING: JWT_SECRET is not set — using an insecure default. " +
			"Set JWT_SECRET to a strong random value; otherwise device/admin tokens are forgeable.")
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

func (s *AuthService) Login(username, password, userAgent, ip string) (string, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return "", errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", errors.New("invalid credentials")
	}

	return s.generateToken(&user, userAgent, ip)
}

func (s *AuthService) generateToken(user *model.User, userAgent, ip string) (string, error) {
	// Generate a session ID (just a random string for now, or use UUID)
	// For simplicity, we'll let the DB allocate an ID, but we need something in the token to link them.
	// Actually, we can create the session first.
	session := model.UserSession{
		UserID:    user.ID,
		UserAgent: userAgent,
		IP:        ip,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // 30 days
		TokenID:   "",                                  // Will be populated after token generation if we use JTI
	}

	if err := s.db.Create(&session).Error; err != nil {
		return "", err
	}

	claims := JWTClaims{
		UserID:   user.ID,
		Username: user.Username,
		KeyID:    session.ID, // Reuse KeyID field for SessionID since it serves same purpose (identifying the key/session)
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(session.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", err
	}

	// Update session with token signature (optional, but good for security)
	// For now, ID is enough.
	return tokenString, nil
}

func (s *AuthService) GenerateDeviceToken(userID uint, username string, name string, deviceID *uint) (string, error) {
	// Create API Key record
	apiKey := model.APIKey{
		UserID:   userID,
		DeviceID: deviceID,
		Name:     name,
	}
	if err := s.db.Create(&apiKey).Error; err != nil {
		return "", err
	}

	var devID uint
	if deviceID != nil {
		devID = *deviceID
	}

	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		KeyID:    apiKey.ID,
		DeviceID: devID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(87600 * time.Hour)), // 10 years
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "device",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) GetOrGenerateDeviceToken(userID uint, username string, name string, deviceID *uint) (string, error) {
	// Look for existing key for this device
	var apiKey model.APIKey
	query := s.db.Where("user_id = ?", userID)
	if deviceID != nil {
		query = query.Where("device_id = ?", *deviceID)
	} else {
		query = query.Where("name = ?", name)
	}
	if err := query.First(&apiKey).Error; err == nil {
		// Reuse existing key — regenerate JWT from it
		var devID uint
		if apiKey.DeviceID != nil {
			devID = *apiKey.DeviceID
		}
		claims := JWTClaims{
			UserID:   userID,
			Username: username,
			KeyID:    apiKey.ID,
			DeviceID: devID,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(87600 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Subject:   "device",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		return token.SignedString(s.jwtSecret)
	}
	// No existing key — create a new one
	return s.GenerateDeviceToken(userID, username, name, deviceID)
}

func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		// If subject is device, check APIKey table
		if claims.Subject == "device" {
			if claims.KeyID > 0 {
				var apiKey model.APIKey
				if err := s.db.First(&apiKey, claims.KeyID).Error; err != nil {
					return nil, errors.New("token revoked")
				}
				// Enrich from DB for legacy tokens (no DeviceID in JWT)
				if claims.DeviceID == 0 && apiKey.DeviceID != nil {
					claims.DeviceID = *apiKey.DeviceID
				}
			}
			return claims, nil
		}

		// Otherwise check UserSession table
		if claims.KeyID > 0 {
			var count int64
			s.db.Model(&model.UserSession{}).Where("id = ?", claims.KeyID).Count(&count)
			if count == 0 {
				return nil, errors.New("session revoked or expired")
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

func (s *AuthService) UpdateTokenDevice(userID uint, tokenID uint, deviceID *uint) error {
	return s.db.Model(&model.APIKey{}).Where("user_id = ? AND id = ?", userID, tokenID).Update("device_id", deviceID).Error
}

func (s *AuthService) RevokeToken(userID uint, tokenID uint) error {
	return s.db.Where("user_id = ? AND id = ?", userID, tokenID).Delete(&model.APIKey{}).Error
}

func (s *AuthService) ListSessions(userID uint) ([]model.UserSession, error) {
	var sessions []model.UserSession
	err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).Find(&sessions).Error
	return sessions, err
}

func (s *AuthService) RevokeSession(userID uint, sessionID uint) error {
	return s.db.Where("user_id = ? AND id = ?", userID, sessionID).Delete(&model.UserSession{}).Error
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
