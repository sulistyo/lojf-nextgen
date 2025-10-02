package bot

import (
	"fmt"
	"net/url"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/events"
	"github.com/lojf/nextgen/internal/models"
)

func init() {
	events.OnPromotion = func(reg models.Registration) {
		// Load related records
		var p models.Parent
		if err := db.Conn().First(&p, reg.ParentID).Error; err != nil { return }
		var c models.Child; _ = db.Conn().First(&c, reg.ChildID).Error
		var cl models.Class; _ = db.Conn().First(&cl, reg.ClassID).Error

		// Find linked Telegram user
		var tu models.TelegramUser
		if err := db.Conn().Where("parent_id = ? AND deliverable = 1", p.ID).First(&tu).Error; err != nil { return }

		loc, _ := time.LoadLocation("Asia/Jakarta")
		dateStr := cl.Date.In(loc).Format("Mon, 02 Jan 2006")

		msg := fmt.Sprintf("🎉 <b>Promoted from Waitlist</b>\n%s — %s — %s\nCode: <code>%s</code>", c.Name, cl.Name, dateStr, reg.Code)
		client := NewClient()
		_ = client.SendMessage(tu.ChatID, msg, nil)
		_ = client.SendPhoto(tu.ChatID, "https://nextgen.lojf.id/qr/"+url.PathEscape(reg.Code)+".png", "", nil)
	}
}
