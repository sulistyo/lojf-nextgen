package bot

import (
	"fmt"
	"os"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func StartReminderLoop() {
	if os.Getenv("TG_ENABLE_REMINDERS") != "1" { return }
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runReminders()
		}
	}()
}

func runReminders() {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	for _, ahead := range []time.Duration{24 * time.Hour, 2 * time.Hour} {
		winStart := now.Add(ahead).Add(-2 * time.Minute)
		winEnd := now.Add(ahead).Add(2 * time.Minute)

		type row struct {
			Parent uint
			Child  string
			Class  string
			Code   string
			Date   time.Time
			Status string
		}
		var rows []row
		db.Conn().Table("registrations r").
			Select(`r.parent_id as parent, children.name as child, classes.name as class,
			        r.code, classes.date as date, r.status`).
			Joins("JOIN children ON children.id = r.child_id").
			Joins("JOIN classes ON classes.id = r.class_id").
			Where("classes.date BETWEEN ? AND ?", winStart, winEnd).
			Where("r.status IN ('confirmed','waitlisted')").
			Scan(&rows)

		for _, x := range rows {
			var tu models.TelegramUser
			if err := db.Conn().Where("parent_id = ? AND deliverable = 1", x.Parent).First(&tu).Error; err != nil { continue }
			c := NewClient()
			dateStr := x.Date.In(loc).Format("Mon, 02 Jan 2006 15:04")
			if x.Status == "waitlisted" {
				_ = c.SendMessage(tu.ChatID, fmt.Sprintf("⏰ Reminder: %s — %s — %s\nStatus: Waitlist", x.Child, x.Class, dateStr), nil)
			} else {
				_ = c.SendMessage(tu.ChatID, fmt.Sprintf("⏰ Reminder: %s — %s — %s\nCode: <code>%s</code>", x.Child, x.Class, dateStr, x.Code), nil)
				_ = c.SendPhoto(tu.ChatID, "https://nextgen.lojf.id/qr/"+x.Code+".png", "", nil)
			}
		}
	}
}
