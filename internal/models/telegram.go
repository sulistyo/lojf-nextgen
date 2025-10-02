package models

import "time"

type TelegramUser struct {
	ID             uint      `gorm:"primarykey"`
	TelegramUserID int64     `gorm:"uniqueIndex"`
	ChatID         int64
	Username       string
	FirstName      string
	Language       string
	Phone          string
	ParentID       *uint
	LinkedAt       *time.Time
	Deliverable    bool `gorm:"default:true"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type LinkCode struct {
	ID        uint      `gorm:"primarykey"`
	Code      string    `gorm:"uniqueIndex"`
	ParentID  uint      `gorm:"index"`
	ExpiresAt time.Time `gorm:"index"`
	UsedAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
