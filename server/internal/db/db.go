package db

import (
	"log"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file" // Import file source driver
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Init(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	log.Println("Database connection established")

	// Auto Migrate Schema
	// Note: We still keep AutoMigrate for other models for now, but `devices` is handled by migration
	err = db.AutoMigrate(
		&model.Setting{},
		&model.Image{},
		&model.GoogleAuth{},
		&model.User{},
		&model.APIKey{},
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func Migrate(db *gorm.DB, dbPath string) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	driver, err := sqlite3.WithInstance(sqlDB, &sqlite3.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"sqlite3", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	log.Println("Database migrations applied successfully")

	return nil
}
