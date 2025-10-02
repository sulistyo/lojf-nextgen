package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

type Dispatcher struct {
	c *Client
}

func ContactKeyboard() any {
	return map[string]any{
		"keyboard": [][]map[string]any{
			{{"text": "Share my phone", "request_contact": true}},
		},
		"resize_keyboard":   true,
		"one_time_keyboard": false,
	}
}

func NewDispatcher() *Dispatcher { return &Dispatcher{c: NewClient()} }

func (d *Dispatcher) Handle(u *Update) {
	// Message
	if u.Message != nil {
		m := u.Message
		chat := m.Chat.ID
		from := m.From

		// Upsert telegram_users
		var tu models.TelegramUser
		_ = db.Conn().Where("telegram_user_id = ?", from.ID).
			FirstOrCreate(&tu, models.TelegramUser{
				TelegramUserID: from.ID,
				ChatID:         chat,
				Username:       from.Username,
				FirstName:      from.FirstName,
				Deliverable:    true,
			}).Error

		// Contact link (no handlers package)
		if m.Contact != nil && m.Contact.UserID == from.ID {
			phone := svc.NormPhone(m.Contact.PhoneNumber)
			tu.Phone = phone

			if p, err := svc.FindParentByAny(phone); err == nil {
			    tu.ParentID = &p.ID
			    now := time.Now()
			    tu.LinkedAt = &now
			    tu.Phone = p.Phone // store canonical phone from DB
			    _ = d.c.SendMessage(chat, fmt.Sprintf("✅ Linked to <b>%s</b> (%s)", p.Name, p.Phone), MainKeyboard())
			} else {
			    _ = d.c.SendMessage(chat, "Phone not found. Send /link CODE from the website ‘Link Telegram’.", MainKeyboard())
			}
			db.Conn().Save(&tu)
			return
		}

		text := strings.TrimSpace(m.Text)
		switch {
		case strings.HasPrefix(text, "/start"):
			_ = d.c.SendMessage(chat,"Hi! Tap the button below to link your account by sharing your phone number.", ContactKeyboard())
			//_ = d.c.SendMessage(chat, "Hi! Use <b>My registrations</b>, <b>Register</b>, <b>Add child</b>, or <b>Account</b>. To link: share your phone or use /link 123456.", MainKeyboard())
		case strings.HasPrefix(text, "/link"):
			code := strings.TrimSpace(strings.TrimPrefix(text, "/link"))
			code = strings.Trim(code, " :")
			d.handleLinkCode(&tu, chat, code)
		case strings.EqualFold(text, "My registrations"), strings.HasPrefix(text, "/my"):
			d.handleMy(chat, &tu)
		case strings.EqualFold(text, "Register"), strings.HasPrefix(text, "/register"):
			d.handleRegisterStart(chat, &tu)
		case strings.EqualFold(text, "Add child"), strings.HasPrefix(text, "/addchild"):
			d.handleAddChildStart(chat, &tu)
		case strings.EqualFold(text, "Account"), strings.HasPrefix(text, "/account"):
			d.handleAccount(chat, &tu)
		default:
			_ = d.c.SendMessage(chat, "Try: <b>My registrations</b> or /help", MainKeyboard())
		}
		return
	}

	// (Inline callback handling can be added later)
}

func MainKeyboard() any {
	return map[string]any{
		"keyboard": [][]map[string]string{
			{{"text": "My registrations"}},
			{{"text": "Register"}},
			{{"text": "Add child"}},
			{{"text": "Account"}},
		},
		"resize_keyboard":    true,
		"one_time_keyboard":  false,
	}
}
