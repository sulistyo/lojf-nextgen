package services

import (
	"sort"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

var (
	ErrDuplicateReg = errors.New("already registered for this class")
	ErrSameDayReg   = errors.New("already registered for another class on that day")
)

// RecomputeClass enforces capacity for a class:
// - checked-in stay confirmed
// - then by CreatedAt (oldest first) until capacity
// others become waitlisted (canceled ignored)
func RecomputeClass(classID uint) error {
	return db.Conn().Transaction(func(tx *gorm.DB) error {
		return RecomputeClassTx(tx, classID)
	})
}

// CancelByCode marks a registration canceled and rebalances its class.
func CancelByCode(code string) error {
	return db.Conn().Transaction(func(tx *gorm.DB) error {
		var reg models.Registration
		if err := tx.Where("code = ?", code).First(&reg).Error; err != nil {
			return err
		}
		if reg.Status == "canceled" {
			return nil
		}
		reg.Status = "canceled"
		reg.CheckInAt = nil
		if err := tx.Save(&reg).Error; err != nil {
			return err
		}
		return RecomputeClassTx(tx, reg.ClassID)
	})
}

// RecomputeClassTx does the same as RecomputeClass but inside an existing TX.
func RecomputeClassTx(tx *gorm.DB, classID uint) error {
	var class models.Class
	if err := tx.First(&class, classID).Error; err != nil {
	 return err
	}

	// non-canceled regs
	var regs []models.Registration
	if err := tx.Where("class_id = ? AND status <> ?", classID, "canceled").
		Find(&regs).Error; err != nil {
		return err
	}

	// order: checked-in first, then by CreatedAt asc
	sort.Slice(regs, func(i, j int) bool {
		ci := regs[i].CheckInAt != nil
		cj := regs[j].CheckInAt != nil
		if ci != cj {
			return ci
		}
		return regs[i].CreatedAt.Before(regs[j].CreatedAt)
	})

	// apply statuses against capacity
	confirmed := 0
	for i := range regs {
		want := "waitlisted"
		if confirmed < class.Capacity {
			want = "confirmed"
		}
		if regs[i].Status != want {
			regs[i].Status = want
			if err := tx.Save(&regs[i]).Error; err != nil {
				return err
			}
		}
		if want == "confirmed" {
			confirmed++
		}
	}
	return nil
}

func CheckRegistrationConflicts(childID, classID uint) error {
	// 1) same class?
	var dup int64
	if err := db.Conn().Model(&models.Registration{}).
		Where("child_id = ? AND class_id = ? AND status IN ?", childID, classID, []string{"confirmed", "waitlisted"}).
		Count(&dup).Error; err != nil {
		return err
	}
	if dup > 0 {
		return ErrDuplicateReg
	}

	// Load class date
	var class models.Class
	if err := db.Conn().First(&class, classID).Error; err != nil {
		return err
	}

	// 2) same day (Asia/Jakarta)
	loc, _ := time.LoadLocation("Asia/Jakarta")
	day := class.Date.In(loc)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	// compare against other classes that day
	var dayCnt int64
	if err := db.Conn().Model(&models.Registration{}).
		Joins("JOIN classes ON classes.id = registrations.class_id").
		Where("registrations.child_id = ? AND registrations.status IN ?", childID, []string{"confirmed", "waitlisted"}).
		Where("classes.date >= ? AND classes.date < ?", start.UTC(), end.UTC()).
		Count(&dayCnt).Error; err != nil {
		return err
	}
	if dayCnt > 0 {
		return ErrSameDayReg
	}

	return nil
}