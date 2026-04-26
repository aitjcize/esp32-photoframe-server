package db

import (
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file" // Import file source driver
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Init(dbPath string) (*gorm.DB, error) {
	// WAL lets readers proceed during writes; busy_timeout makes the few
	// remaining writer/writer collisions wait instead of failing with
	// "database is locked". Without these, the async device_history writer
	// in ServeImage routinely blocks user-visible writes (gallery delete,
	// settings save) for tens of seconds on a busy server.
	//
	// 30s is the conventional SQLite "be patient" budget — long enough to
	// outlast a multi-thousand-photo Synology / Immich sync that's writing
	// one row at a time, short enough that a real deadlock still surfaces.
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	log.Println("Database connection established")

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
