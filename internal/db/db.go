package db

import (
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/lojf/nextgen/internal/models"
)

var conn *gorm.DB

func Init() error {
	var err error
	conn, err = gorm.Open(sqlite.Open("nextgen.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		return err
	}

	// SQLite works best with a single writer; cap the pool accordingly.
	sqlDB, err := conn.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	// AutoMigrate core tables

	if err := conn.AutoMigrate(
		&models.Parent{},
		&models.Child{},
		&models.Class{},
		&models.Registration{},
		&models.TelegramUser{},       
		&models.LinkCode{},           
		&models.ClassQuestion{}, 
		&models.ClassTemplate{}, 
		&models.ClassTemplateQuestion{},
		&models.RegistrationAnswer{}, 
	); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}

	// Composite indexes that GORM doesn't auto-create from struct tags.
	conn.Exec("CREATE INDEX IF NOT EXISTS idx_reg_class_status ON registrations(class_id, status)")
	conn.Exec("CREATE INDEX IF NOT EXISTS idx_reg_parent      ON registrations(parent_id)")

	log.Println("database ready (sqlite)")
	return nil
}

func Conn() *gorm.DB {
	return conn
}
