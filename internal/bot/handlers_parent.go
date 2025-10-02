package bot

import (
	"fmt"
	"net/url"
	"time"
	"strings"
	"unicode"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (d *Dispatcher) handleLinkCode(tu *models.TelegramUser, chat int64, code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		_ = d.c.SendMessage(chat, "Use: /link 123456\nOpen the website → Account → Link Telegram to get a code.", nil)
		return
	}
	code = onlyDigits(code) // strip spaces, punctuation, accidental chars

	var lc models.LinkCode
	err := db.Conn().Where("code = ? AND used_at IS NULL AND expires_at > ?", code, time.Now()).
		First(&lc).Error
	if err != nil {
		_ = d.c.SendMessage(chat, "Code invalid or expired.", nil)
		return
	}
	now := time.Now()
	lc.UsedAt = &now
	_ = db.Conn().Save(&lc).Error

	var p models.Parent
	if err := db.Conn().First(&p, lc.ParentID).Error; err != nil {
		_ = d.c.SendMessage(chat, "Parent not found.", nil)
		return
	}
	tu.ParentID = &p.ID
	tu.Phone = p.Phone
	tu.LinkedAt = &now
	_ = db.Conn().Save(tu).Error

	_ = d.c.SendMessage(chat, fmt.Sprintf("✅ Linked to <b>%s</b> (%s)", p.Name, p.Phone), MainKeyboard())
}

func (d *Dispatcher) handleMy(chat int64, tu *models.TelegramUser) {
	if tu.ParentID == nil {
		_ = d.c.SendMessage(chat, "Not linked yet. Share your phone or use /link CODE.", nil); return
	}
	type row struct {
		Code string
		Status string
		Child string
		Class string
		Date time.Time
	}
	var rows []row
	db.Conn().Table("registrations r").
		Select("r.code, r.status, children.name as child, classes.name as class, classes.date as date").
		Joins("JOIN children ON children.id = r.child_id").
		Joins("JOIN classes ON classes.id = r.class_id").
		Where("r.parent_id = ? AND classes.date >= ?", *tu.ParentID, time.Now().Add(-2*time.Hour)).
		Order("classes.date asc, r.created_at asc").
		Scan(&rows)

	if len(rows)==0 {
		_ = d.c.SendMessage(chat, "No upcoming registrations.", nil); return
	}

	var b strings.Builder
	b.WriteString("<b>Your upcoming registrations</b>\n")
	loc, _ := time.LoadLocation("Asia/Jakarta")
	for _, r := range rows {
		date := r.Date.In(loc).Format("Mon, 02 Jan 2006")
		if r.Status == "waitlisted" {
			fmt.Fprintf(&b, "• %s — %s — %s — Waitlist\n", date, r.Class, r.Child)
		} else {
			fmt.Fprintf(&b, "• %s — %s — %s — <code>%s</code>\n", date, r.Class, r.Child, r.Code)
		}
	}
	b.WriteString("\nTap /qr CODE to show QR or /cancel CODE to cancel.")
	_ = d.c.SendMessage(chat, b.String(), nil)
}

func (d *Dispatcher) handleRegisterStart(chat int64, tu *models.TelegramUser) {
	if tu.ParentID == nil {
		_ = d.c.SendMessage(chat, "Link first (share phone or /link CODE).", nil); return
	}
	// For MVP, deep-link to your site flow:
	_ = d.c.SendMessage(chat, "Open the registration page:", map[string]any{
		"inline_keyboard": [][]map[string]any{
			{{"text":"Open Register","url":"https://nextgen.lojf.id/register?k=1"}},
		},
	})
}

func (d *Dispatcher) handleAddChildStart(chat int64, tu *models.TelegramUser) {
	if tu.ParentID == nil {
		_ = d.c.SendMessage(chat, "Link first (share phone or /link CODE).", nil); return
	}
	_ = d.c.SendMessage(chat, "Add child on the website:", map[string]any{
		"inline_keyboard": [][]map[string]any{
			{{"text":"Open My Account","url":"https://nextgen.lojf.id/account/profile"}},
		},
	})
}

func (d *Dispatcher) handleAccount(chat int64, tu *models.TelegramUser) {
	if tu.ParentID == nil {
		_ = d.c.SendMessage(chat, "Not linked. Share phone or /link CODE.", nil); return
	}
	u := "https://nextgen.lojf.id/my/list"
	_ = d.c.SendMessage(chat, "Open your account & registrations:", map[string]any{
		"inline_keyboard": [][]map[string]any{
			{{"text":"My Registrations","url":u}},
			{{"text":"Account Profile","url":"https://nextgen.lojf.id/account/profile"}},
		},
	})
}

// Public helpers for other packages
func NotifyPromotion(parentID uint, childName, className, dateStr, code string) {
	var tu models.TelegramUser
	if err := db.Conn().Where("parent_id = ? AND deliverable = 1", parentID).First(&tu).Error; err != nil { return }
	c := NewClient()
	msg := fmt.Sprintf("🎉 <b>Promoted from Waitlist</b>\n%s — %s — %s\nCode: <code>%s</code>", childName, className, dateStr, code)
	_ = c.SendMessage(tu.ChatID, msg, nil)
	_ = c.SendPhoto(tu.ChatID, "https://nextgen.lojf.id/qr/"+url.PathEscape(code)+".png", "", nil)
}
