package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/events"
	"github.com/lojf/nextgen/internal/models"
)

var (
	ErrDuplicateReg = errors.New("already registered for this class")
	ErrSameDayReg   = errors.New("already registered for another class on that day")
)

// RecomputeClass enforces capacity and, if anyone is promoted from waitlist → confirmed,
// triggers promotion events AFTER the transaction commits.
func RecomputeClass(classID uint) error {
	var promoted []models.Registration
	err := db.Conn().Transaction(func(tx *gorm.DB) error {
		var err error
		promoted, err = recomputeClassTxCollect(tx, classID)
		return err
	})
	if err != nil {
		return err
	}
	notifyPromotions(promoted) // outside the TX
	return nil
}

// CancelByCode marks a registration canceled, rebalances, and triggers promotion events.
func CancelByCode(code string) error {
	var promoted []models.Registration
	err := db.Conn().Transaction(func(tx *gorm.DB) error {
		var reg models.Registration
		if err := tx.Where("code = ?", code).First(&reg).Error; err != nil {
			return err
		}
		if reg.Status != "canceled" {
			reg.Status = "canceled"
			reg.CheckInAt = nil
			if err := tx.Save(&reg).Error; err != nil {
				return err
			}
		}
		var err error
		promoted, err = recomputeClassTxCollect(tx, reg.ClassID)
		return err
	})
	if err != nil {
		return err
	}
	notifyPromotions(promoted)
	return nil
}

// RecomputeClassTx is kept for callers inside an existing TX (no events here).
func RecomputeClassTx(tx *gorm.DB, classID uint) error {
	_, err := recomputeClassTxCollect(tx, classID)
	return err
}


func recomputeClassTxCollect(tx *gorm.DB, classID uint) ([]models.Registration, error) {
    var class models.Class
    if err := tx.First(&class, classID).Error; err != nil {
        return nil, err
    }

    // 1. Load confirmed (FIFO preserved)
    var confirmed []models.Registration
    if err := tx.
        Where("class_id = ? AND status = 'confirmed'", classID).
        Order("created_at asc, id asc").
        Find(&confirmed).Error; err != nil {
        return nil, err
    }

    // 2. Load waitlist FIFO
    var waitlist []models.Registration
    if err := tx.
        Where("class_id = ? AND status = 'waitlisted'", classID).
        Order("created_at asc, id asc").
        Find(&waitlist).Error; err != nil {
        return nil, err
    }

    promoted := []models.Registration{}

    // 3. Promote oldest waitlisted until capacity filled
    slots := class.Capacity - len(confirmed)
    if slots > 0 {
        for i := 0; i < slots && i < len(waitlist); i++ {
            waitlist[i].Status = "confirmed"
            if err := tx.Save(&waitlist[i]).Error; err != nil {
                return nil, err
            }
            promoted = append(promoted, waitlist[i])
        }
    }

    return promoted, nil
}


// internal: fire events for promotions
func notifyPromotions(promoted []models.Registration) {
	if len(promoted) == 0 || events.OnPromotion == nil {
		return
	}
	for _, r := range promoted {
		events.OnPromotion(r)
	}
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
