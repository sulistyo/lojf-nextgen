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
	ID        uint      `gorm:"primaryKey"`
	ParentID  uint      `gorm:"index"`
	Name      string
	BirthDate time.Time
	// NEW:
	Gender    string     // "", "Boy", "Girl", "Other" (free text allowed)
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Class struct {
	ID        uint       `gorm:"primaryKey"`
	Name      string
	Date      time.Time
	Capacity  int
	// NEW:
	Description    string       `gorm:"type:text"`
	SignupOpensAt  *time.Time   // nil = open now


	CreatedAt time.Time
	UpdatedAt time.Time
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

type ClassQuestion struct {
	ID         uint   `gorm:"primaryKey"`
	ClassID    *uint  `gorm:"index"`  // pointer (nil if it belongs to a template)
	TemplateID *uint  `gorm:"index"`  // pointer (nil if it belongs to a class)

	Label    string
	Kind     string // "text" | "radio"
	Options  string `gorm:"column:choices"` // <-- match your code: q.Options; stored in 'choices' column
	Required bool
	Position int

	CreatedAt time.Time
	UpdatedAt time.Time
}

type ClassTemplate struct {
	ID          uint      `gorm:"primaryKey"`
	Name        string    `gorm:"size:200;not null"`
	Description string    `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Questions   []ClassTemplateQuestion `gorm:"foreignKey:TemplateID"`
}

type ClassTemplateQuestion struct {
	ID         uint      `gorm:"primaryKey"`
	TemplateID uint      `gorm:"index;not null"`
	Label      string    `gorm:"size:255;not null"`
	Kind       string    `gorm:"size:20;not null"` // "text" or "radio"
	Options    string    `gorm:"type:text"`        // radio options (comma-separated or newline—your choice)
	Required   bool      `gorm:"not null"`
	Position   int       `gorm:"not null;default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type RegistrationAnswer struct {
	ID             uint      `gorm:"primaryKey"`
	RegistrationID uint      `gorm:"index;not null"`
	QuestionID     uint      `gorm:"index;not null"`
	Answer         string    `gorm:"type:TEXT;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
