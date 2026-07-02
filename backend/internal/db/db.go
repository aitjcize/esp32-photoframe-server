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
	// _foreign_keys=on enforces the ON DELETE CASCADE / SET NULL constraints on
	// the junction/child tables (see migration 000032). SQLite defaults this OFF
	// per-connection; with SetMaxOpenConns(1) below it's set once on the single
	// connection and every query inherits it.
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=30000&_synchronous=NORMAL&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Serialize all access through a single connection. SQLite allows only one
	// writer, and with an unbounded pool the concurrent startup syncs (all four
	// sources import at once) open multiple connections whose write transactions
	// collide. That collision is the WAL writer-upgrade case, which returns
	// SQLITE_BUSY ("database is locked") immediately WITHOUT honoring
	// busy_timeout — so the DSN pragma alone doesn't prevent it. One connection
	// turns writer/writer contention into in-process queueing; reads here are
	// sub-millisecond so serializing them is cheap.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

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

	upErr := m.Up()
	if upErr != nil && upErr != migrate.ErrNoChange {
		return upErr
	}

	log.Println("Database migrations applied successfully")

	// If a migration actually ran, VACUUM to reclaim pages freed by table
	// rebuilds and orphan purges (e.g. 000032 rebuilds five tables and deletes
	// dangling rows). VACUUM can't run inside a transaction, so it can't live in
	// a migration file (golang-migrate wraps each one in a tx); running it here,
	// after Up() commits, keeps it out of any transaction. Gated on ErrNoChange
	// so it's a one-time cost per upgrade rather than on every boot. Non-fatal:
	// a failed VACUUM shouldn't stop the server from starting.
	if upErr != migrate.ErrNoChange {
		if err := db.Exec("VACUUM").Error; err != nil {
			log.Printf("VACUUM after migration failed (non-fatal): %v", err)
		} else {
			log.Println("VACUUM complete: reclaimed free pages")
		}
	}

	return nil
}
