package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.APIKey{}, &model.UserSession{}))
	return db
}

// TestAuthService_TokenLifecycle covers register/login, session + device token
// validation, secret enforcement, and revocation — the security-critical paths
// that previously had no test coverage.
func TestAuthService_TokenLifecycle(t *testing.T) {
	db := setupAuthDB(t)
	svc := NewAuthService(db, "secret-a")

	require.NoError(t, svc.Register("admin", "pw"))
	assert.Error(t, svc.Register("admin", "pw2"), "duplicate username must be rejected")

	// Login: good and bad credentials.
	sessTok, err := svc.Login("admin", "pw", "ua", "1.2.3.4")
	require.NoError(t, err)
	_, err = svc.Login("admin", "wrong", "ua", "1.2.3.4")
	assert.Error(t, err, "wrong password must fail")

	// Session token validates and carries the username.
	claims, err := svc.ValidateToken(sessTok)
	require.NoError(t, err)
	assert.Equal(t, "admin", claims.Username)

	var user model.User
	require.NoError(t, db.Where("username = ?", "admin").First(&user).Error)

	// Device token carries device id + "device" subject.
	devID := uint(7)
	devTok, err := svc.GenerateDeviceToken(user.ID, "admin", "frame", &devID)
	require.NoError(t, err)
	devClaims, err := svc.ValidateToken(devTok)
	require.NoError(t, err)
	assert.Equal(t, devID, devClaims.DeviceID)
	assert.Equal(t, "device", devClaims.Subject)

	// A token signed with a different secret must not validate.
	svcB := NewAuthService(db, "secret-b")
	_, err = svcB.ValidateToken(devTok)
	assert.Error(t, err, "token signed with secret-a must not validate under secret-b")

	// Revoking the API key invalidates the device token.
	require.NoError(t, svc.RevokeToken(user.ID, devClaims.KeyID))
	_, err = svc.ValidateToken(devTok)
	assert.Error(t, err, "revoked device token must fail validation")

	// Revoking the session invalidates the session token.
	require.NoError(t, svc.RevokeSession(user.ID, claims.KeyID))
	_, err = svc.ValidateToken(sessTok)
	assert.Error(t, err, "revoked session token must fail validation")
}

// TestAuthService_RejectsNonHS256 verifies the alg-confusion guard
// (jwt.WithValidMethods) — an alg=none token must be rejected.
func TestAuthService_RejectsNonHS256(t *testing.T) {
	db := setupAuthDB(t)
	svc := NewAuthService(db, "secret-a")

	claims := JWTClaims{
		UserID:   1,
		Username: "x",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = svc.ValidateToken(signed)
	assert.Error(t, err, "alg=none token must be rejected")
}
