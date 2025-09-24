package services

import (
	"sort"

	"gorm.io/gorm"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
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