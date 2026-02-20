package bot

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func StartReminderLoop() {
	if os.Getenv("TG_ENABLE_REMINDERS") != "1" {
		return
	}
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runReminders()
		}
	}()
}

// Parse REMIND_OFFSETS like "24h,2h,1h". Defaults to 24h & 2h.
func parseOffsets() []time.Duration {
	raw := strings.TrimSpace(os.Getenv("REMIND_OFFSETS"))
	if raw == "" {
		return []time.Duration{24 * time.Hour, 2 * time.Hour}
	}
	parts := strings.Split(raw, ",")
	out := make([]time.Duration, 0, len(parts))
	for _, p := range parts {
		d, err := time.ParseDuration(strings.TrimSpace(p))
		if err == nil && d > 0 {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		out = []time.Duration{24 * time.Hour, 2 * time.Hour}
	}
	return out
}

var remindersLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}()

func runReminders() {
	loc := remindersLoc
	now := time.Now().In(loc)
	// Use a strict 1-minute window: [tick, tick+1m) to avoid duplicate sends
	tick := now.Truncate(time.Minute)
	next := tick.Add(time.Minute)

	offsets := parseOffsets()
	includeWaitlist := os.Getenv("REMIND_INCLUDE_WAITLIST") == "1"

	for _, ahead := range offsets {
		// Classes due to remind in this tick:
		// trigger = class.date - ahead ∈ [tick, next)
		// => class.date ∈ [tick+ahead, next+ahead)
		start := tick.Add(ahead)
		end := next.Add(ahead)

		type row struct {
			Parent uint
			Child  string
			Class  string
			Code   string
			Date   time.Time
			Status string
		}
		var rows []row

		q := db.Conn().Table("registrations r").
			Select(`r.parent_id as parent,
			        children.name as child,
			        classes.name  as class,
			        r.code,
			        classes.date  as date,
			        r.status`).
			Joins("JOIN children ON children.id = r.child_id").
			Joins("JOIN classes  ON classes.id = r.class_id").
			Where("classes.date >= ? AND classes.date < ?", start, end)

		if includeWaitlist {
			q = q.Where("r.status IN ('confirmed','waitlisted')")
		} else {
			q = q.Where("r.status = 'confirmed'")
		}

		if err := q.Scan(&rows).Error; err != nil {
			continue
		}

		if len(rows) == 0 {
			continue
		}

		// Batch-load TelegramUsers for all parent IDs in one query.
		parentIDs := make([]uint, 0, len(rows))
		for _, x := range rows {
			parentIDs = append(parentIDs, x.Parent)
		}
		var tgUsers []models.TelegramUser
		_ = db.Conn().Where("parent_id IN ? AND deliverable = 1", parentIDs).Find(&tgUsers).Error

		tgMap := make(map[uint]models.TelegramUser, len(tgUsers))
		for _, tu := range tgUsers {
			if tu.ParentID != nil {
				tgMap[*tu.ParentID] = tu
			}
		}

		c := NewClient()
		for _, x := range rows {
			tu, ok := tgMap[x.Parent]
			if !ok {
				continue
			}
			dateStr := x.Date.In(loc).Format("Mon, 02 Jan 2006 15:04")

			if x.Status == "waitlisted" {
				_ = c.SendMessage(tu.ChatID,
					fmt.Sprintf("⏰ Reminder: %s — %s — %s\nStatus: Waitlist", x.Child, x.Class, dateStr),
					nil)
				continue
			}

			// Confirmed: code + QR
			_ = c.SendMessage(tu.ChatID,
				fmt.Sprintf("⏰ Reminder: %s — %s — %s\nCode: <code>%s</code>", x.Child, x.Class, dateStr, x.Code),
				nil)
			_ = c.SendPhoto(tu.ChatID,
				"https://nextgen.lojf.id/qr/"+x.Code+".png", "", nil)
		}
	}
}
