package models

import "time"

// Role values for AdminUser.
const (
	RoleAdmin   = "admin"   // full access: classes, templates, parents, cancel/delete
	RoleCheckin = "checkin" // check-in station only, scoped to today + own campus
)

// AdminUser is a login account for the admin area.
//
// Two tiers, on purpose:
//   - RoleAdmin accounts are per-person (a handful of stable leaders).
//   - RoleCheckin accounts are generic and shared per campus (e.g. "fjb-checkin"),
//     because volunteers rotate weekly and cannot wait for provisioning.
//
// Campus is only meaningful for RoleCheckin: it pins the account to classes whose
// name starts with that prefix (see CampusOf). Empty campus = all campuses.
type AdminUser struct {
	ID        uint   `gorm:"primaryKey"`
	Username  string `gorm:"uniqueIndex;not null"`
	PassHash  string `gorm:"not null"`
	Role      string `gorm:"index;not null"`
	Campus    string `gorm:"index"` // "FJB", "FJU", or "" for all
	Active    bool   `gorm:"not null;default:true"`
	LastLogin *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AuditLog records every mutating admin action.
//
// With shared credentials, Username alone cannot identify a person, so check-in
// actions also carry StaffedBy: the free-text name the volunteer types once at
// the start of their shift. It is not authentication, it is bookkeeping.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`

	Username  string `gorm:"index"` // account used
	Role      string
	StaffedBy string // volunteer name for check-in shifts, "" otherwise
	IP        string

	Action string `gorm:"index"` // e.g. "registration.checkin", "class.delete"
	Target string // e.g. "registration:3124 (REG-38EAD13B)"
	Detail string
}

// AppSetting is a tiny key/value store for values that must survive restarts but
// do not deserve their own table (currently just the session signing secret).
type AppSetting struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}
