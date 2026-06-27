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
	svc := NewAuthService(db, "secret-a", false)

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
	svcB := NewAuthService(db, "secret-b", false)
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
	svc := NewAuthService(db, "secret-a", false)

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

// TestAuthService_LegacyDeviceFallback verifies the migration bridge: a DEVICE
// token signed with the old hard-coded default validates under a new secret when
// the fallback is enabled, but a SESSION token signed with the legacy secret
// never does, and nothing legacy validates when the fallback is off.
func TestAuthService_LegacyDeviceFallback(t *testing.T) {
	db := setupAuthDB(t)
	require.NoError(t, NewAuthService(db, "x", false).Register("admin", "pw"))
	var user model.User
	require.NoError(t, db.Where("username = ?", "admin").First(&user).Error)

	// Sign tokens with the legacy default secret (as an old build would).
	legacySigner := NewAuthService(db, legacyDefaultSecret, false)
	devID := uint(3)
	legacyDevTok, err := legacySigner.GenerateDeviceToken(user.ID, "admin", "frame", &devID)
	require.NoError(t, err)
	legacySessTok, err := legacySigner.Login("admin", "pw", "ua", "ip")
	require.NoError(t, err)

	// New secret WITH fallback: legacy device token accepted, legacy session not.
	withFallback := NewAuthService(db, "new-random-secret", true)
	claims, err := withFallback.ValidateToken(legacyDevTok)
	require.NoError(t, err, "legacy device token must validate under the fallback")
	assert.Equal(t, devID, claims.DeviceID)
	_, err = withFallback.ValidateToken(legacySessTok)
	assert.Error(t, err, "legacy session token must NOT validate (no admin forgery)")

	// New secret WITHOUT fallback: legacy device token rejected too.
	noFallback := NewAuthService(db, "new-random-secret", false)
	_, err = noFallback.ValidateToken(legacyDevTok)
	assert.Error(t, err, "with fallback off, legacy device token must be rejected")
}

// TestAuthService_RotateSecret verifies rotation invalidates existing tokens,
// persists the new secret, and disables the legacy fallback.
func TestAuthService_RotateSecret(t *testing.T) {
	db := setupAuthDB(t)
	require.NoError(t, NewAuthService(db, "x", false).Register("admin", "pw"))
	var user model.User
	require.NoError(t, db.Where("username = ?", "admin").First(&user).Error)

	svc := NewAuthService(db, "old-secret", true)
	devID := uint(5)
	oldTok, err := svc.GenerateDeviceToken(user.ID, "admin", "frame", &devID)
	require.NoError(t, err)
	_, err = svc.ValidateToken(oldTok)
	require.NoError(t, err)

	var captured string
	require.NoError(t, svc.RotateSecret(func(secret string) error {
		captured = secret
		return nil
	}))
	assert.Len(t, captured, 64, "rotated secret should be 32 random bytes as hex")

	// Old token no longer validates (secret changed + legacy fallback now off).
	_, err = svc.ValidateToken(oldTok)
	assert.Error(t, err, "tokens issued before rotation must be invalidated")

	// A token minted after rotation validates.
	newTok, err := svc.GenerateDeviceToken(user.ID, "admin", "frame2", &devID)
	require.NoError(t, err)
	_, err = svc.ValidateToken(newTok)
	require.NoError(t, err, "tokens minted after rotation must validate")
}

// TestRotateSecret_PersistFailureKeepsOldSecret ensures a storage failure during
// rotation leaves the running secret intact (persist-first ordering).
func TestRotateSecret_PersistFailureKeepsOldSecret(t *testing.T) {
	db := setupAuthDB(t)
	require.NoError(t, NewAuthService(db, "x", false).Register("admin", "pw"))
	var user model.User
	require.NoError(t, db.Where("username = ?", "admin").First(&user).Error)

	svc := NewAuthService(db, "old-secret", false)
	devID := uint(9)
	tok, err := svc.GenerateDeviceToken(user.ID, "admin", "frame", &devID)
	require.NoError(t, err)

	err = svc.RotateSecret(func(string) error { return assert.AnError })
	require.Error(t, err)

	_, err = svc.ValidateToken(tok)
	require.NoError(t, err, "secret must be unchanged when persistence fails")
}
