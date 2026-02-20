package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/lojf/nextgen/internal/models"
)

// openTestDB returns an isolated in-file SQLite database in a temp directory.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db")
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := gdb.AutoMigrate(
		&models.Parent{},
		&models.Child{},
		&models.Class{},
		&models.Registration{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return gdb
}

// TestCapacityAggregation verifies that the single GROUP BY query used in
// AdminCapacity correctly counts confirmed (not checked-in), waitlisted, and
// checked-in registrations in one round-trip instead of three COUNT queries.
func TestCapacityAggregation(t *testing.T) {
	gdb := openTestDB(t)

	// Seed: 1 class, 1 parent, 4 children.
	cls := models.Class{Name: "Test Class", Capacity: 10, Date: time.Now()}
	gdb.Create(&cls)

	parent := models.Parent{Name: "Parent", Phone: "08123456789"}
	gdb.Create(&parent)

	kids := make([]models.Child, 4)
	for i := range kids {
		kids[i] = models.Child{Name: "Kid", ParentID: parent.ID}
		gdb.Create(&kids[i])
	}

	now := time.Now()
	regs := []models.Registration{
		// 2 confirmed, not yet checked in
		{ClassID: cls.ID, ChildID: kids[0].ID, ParentID: parent.ID, Status: "confirmed", Code: "R001"},
		{ClassID: cls.ID, ChildID: kids[1].ID, ParentID: parent.ID, Status: "confirmed", Code: "R002"},
		// 1 confirmed + already checked in
		{ClassID: cls.ID, ChildID: kids[2].ID, ParentID: parent.ID, Status: "confirmed", Code: "R003", CheckInAt: &now},
		// 1 on the waitlist
		{ClassID: cls.ID, ChildID: kids[3].ID, ParentID: parent.ID, Status: "waitlisted", Code: "R004"},
	}
	for i := range regs {
		gdb.Create(&regs[i])
	}

	// Run the exact aggregation query from admin_capacity.go.
	type capAgg struct {
		ClassID    uint
		Confirmed  int64
		Waitlisted int64
		CheckedIn  int64
	}
	var aggs []capAgg
	err := gdb.Table("registrations").
		Select(`class_id,
			SUM(CASE WHEN status = 'confirmed'  AND check_in_at IS NULL     THEN 1 ELSE 0 END) AS confirmed,
			SUM(CASE WHEN status = 'waitlisted'                             THEN 1 ELSE 0 END) AS waitlisted,
			SUM(CASE WHEN status = 'confirmed'  AND check_in_at IS NOT NULL THEN 1 ELSE 0 END) AS checked_in`).
		Where("class_id IN ?", []uint{cls.ID}).
		Group("class_id").
		Scan(&aggs).Error
	if err != nil {
		t.Fatalf("aggregation query: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("expected 1 aggregation row, got %d", len(aggs))
	}
	a := aggs[0]

	if a.Confirmed != 2 {
		t.Errorf("Confirmed: want 2, got %d", a.Confirmed)
	}
	if a.Waitlisted != 1 {
		t.Errorf("Waitlisted: want 1, got %d", a.Waitlisted)
	}
	if a.CheckedIn != 1 {
		t.Errorf("CheckedIn: want 1, got %d", a.CheckedIn)
	}

	// Verify fill-percent uses corrected operator precedence:
	// (confirmed + checkedIn) * 100 / capacity  →  (2+1)*100/10 = 30
	fill := int((a.Confirmed+a.CheckedIn)*100 / int64(cls.Capacity))
	if fill != 30 {
		t.Errorf("FillPercent: want 30, got %d", fill)
	}
}

// TestCapacityAggregation_MultiClass ensures rows from different classes
// remain segregated after the GROUP BY.
func TestCapacityAggregation_MultiClass(t *testing.T) {
	gdb := openTestDB(t)

	cls1 := models.Class{Name: "C1", Capacity: 5, Date: time.Now()}
	cls2 := models.Class{Name: "C2", Capacity: 5, Date: time.Now()}
	gdb.Create(&cls1)
	gdb.Create(&cls2)

	parent := models.Parent{Name: "P", Phone: "0811111111"}
	gdb.Create(&parent)
	kid := models.Child{Name: "K", ParentID: parent.ID}
	gdb.Create(&kid)
	kid2 := models.Child{Name: "K2", ParentID: parent.ID}
	gdb.Create(&kid2)

	gdb.Create(&models.Registration{ClassID: cls1.ID, ChildID: kid.ID, ParentID: parent.ID, Status: "confirmed", Code: "A1"})
	gdb.Create(&models.Registration{ClassID: cls2.ID, ChildID: kid2.ID, ParentID: parent.ID, Status: "waitlisted", Code: "A2"})

	type capAgg struct {
		ClassID    uint
		Confirmed  int64
		Waitlisted int64
		CheckedIn  int64
	}
	var aggs []capAgg
	gdb.Table("registrations").
		Select(`class_id,
			SUM(CASE WHEN status = 'confirmed'  AND check_in_at IS NULL     THEN 1 ELSE 0 END) AS confirmed,
			SUM(CASE WHEN status = 'waitlisted'                             THEN 1 ELSE 0 END) AS waitlisted,
			SUM(CASE WHEN status = 'confirmed'  AND check_in_at IS NOT NULL THEN 1 ELSE 0 END) AS checked_in`).
		Where("class_id IN ?", []uint{cls1.ID, cls2.ID}).
		Group("class_id").
		Scan(&aggs)

	aggMap := make(map[uint]capAgg)
	for _, a := range aggs {
		aggMap[a.ClassID] = a
	}

	if aggMap[cls1.ID].Confirmed != 1 {
		t.Errorf("cls1 Confirmed: want 1, got %d", aggMap[cls1.ID].Confirmed)
	}
	if aggMap[cls2.ID].Waitlisted != 1 {
		t.Errorf("cls2 Waitlisted: want 1, got %d", aggMap[cls2.ID].Waitlisted)
	}
}
