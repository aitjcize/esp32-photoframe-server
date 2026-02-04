package service

import (
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	db.AutoMigrate(&model.Setting{})
	return db
}

func TestSettingsService_SetGet(t *testing.T) {
	db := setupTestDB()
	svc := NewSettingsService(db)

	err := svc.Set("foo", "bar")
	assert.NoError(t, err)

	val, err := svc.Get("foo")
	assert.NoError(t, err)
	assert.Equal(t, "bar", val)
}

func TestSettingsService_Update(t *testing.T) {
	db := setupTestDB()
	svc := NewSettingsService(db)

	svc.Set("foo", "bar")
	svc.Set("foo", "baz")

	val, err := svc.Get("foo")
	assert.NoError(t, err)
	assert.Equal(t, "baz", val)
}
