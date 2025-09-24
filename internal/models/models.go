package models

import "time"

type Parent struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name  string
	Phone string `gorm:"uniqueIndex;not null"` // unique parent identity
	Email string

	Children []Child
}

type Child struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name      string
	BirthDate time.Time

	ParentID uint
	Parent   Parent
}

type Class struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Date     time.Time
	Name     string
	Capacity int
}

// Status: "confirmed", "waitlisted", "canceled"
type Registration struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ParentID uint
	ChildID  uint
	ClassID  uint

	Status    string     // confirmed | waitlisted | canceled
	Code      string     `gorm:"uniqueIndex"` // e.g., REG-123456
	CheckInAt *time.Time // nil until checked-in
}
