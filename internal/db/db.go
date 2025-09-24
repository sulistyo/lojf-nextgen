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
	conn, err = gorm.Open(sqlite.Open("nextgen.db"), &gorm.Config{})
	if err != nil {
		return err
	}
	// AutoMigrate core tables
	if err := conn.AutoMigrate(&models.Parent{}, &models.Child{}, &models.Class{}, &models.Registration{}); err != nil {
		return err
	}
	log.Println("database ready (sqlite)")
	return nil
}

func Conn() *gorm.DB {
	return conn
}
