// internal/handlers/my.go
package handlers

import (
	"html/template"
	"net/http"
	"time"
	"strings"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

type myRow struct {
	Code      string
	Status    string
	ClassName string
	ClassDate time.Time
	DateStr   string
	ChildName string
}

// GET /my  (optional ?phone=...)
func MyPhoneForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If we already know the parent, skip the phone gate
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			http.Redirect(w, r, "/my/list", http.StatusSeeOther)
			return
		}
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		var parent *models.Parent
		if phone != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil {
				parent = &p
			}
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/my_phone.tmpl"); err != nil { http.Error(w, err.Error(), 500); return }
		if err := view.ExecuteTemplate(w, "parents/my_phone.tmpl", map[string]any{
			"Title":  "My Registrations",
			"Phone":  phone,
			"Parent": parent,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}

// GET /my/list?phone=...
func MyList(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		if strings.TrimSpace(phone) == "" {
			if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
				phone = cPhone
			}
		}
		if strings.TrimSpace(phone) == "" {
			// no cookie and no query → send to phone gate
			http.Redirect(w, r, "/my", http.StatusSeeOther)
			return
		}

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		now := time.Now().Add(-2 * time.Hour)

		type row struct {
			Code      string
			Status    string
			ClassName string
			ClassDate time.Time
			ChildName string
		}
		var rows []row
		db.Conn().Table("registrations").
			Select(`registrations.code, registrations.status,
			        classes.name as class_name, classes.date as class_date,
			        children.name as child_name`).
			Joins("JOIN classes ON classes.id = registrations.class_id").
			Joins("JOIN children ON children.id = registrations.child_id").
			Where("registrations.parent_id = ? AND classes.date >= ?", parent.ID, now).
			Order("classes.date asc, children.name asc").
			Scan(&rows)

		out := make([]myRow, 0, len(rows))
		for _, rrow := range rows {
			out = append(out, myRow{
				Code:      rrow.Code,
				Status:    rrow.Status,
				ClassName: rrow.ClassName,
				ClassDate: rrow.ClassDate,
				DateStr:   fmtDate(rrow.ClassDate),
				ChildName: rrow.ChildName,
			})
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/my_list.tmpl"); err != nil { http.Error(w, err.Error(), 500); return }
		if err := view.ExecuteTemplate(w, "parents/my_list.tmpl", map[string]any{
			"Title":  "My Registrations",
			"Phone":  phone,
			"Rows":   out,
			"Parent": parent,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}

// GET /my/qr?code=REG-xxxxxx
func MyQR(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if strings.TrimSpace(phone) == "" {
			if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
				phone = cPhone
			}
		}
		if strings.TrimSpace(phone) == "" {
			http.Redirect(w, r, "/my", http.StatusSeeOther)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		// Load parent
		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", http.StatusNotFound)
			return
		}

		// Registration must belong to this parent
		var reg models.Registration
		if err := db.Conn().Where("code = ? AND parent_id = ?", code, parent.ID).First(&reg).Error; err != nil {
			http.NotFound(w, r)
			return
		}

		var child models.Child
		_ = db.Conn().First(&child, reg.ChildID).Error
		var class models.Class
		_ = db.Conn().First(&class, reg.ClassID).Error

		// If waitlisted, compute rank = number of waitlisted regs for this class
		// with created_at <= this reg's created_at (FIFO).
		waitRank := 0
		if reg.Status == "waitlisted" {
			var cnt int64
			_ = db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ? AND created_at <= ?",
					reg.ClassID, "waitlisted", reg.CreatedAt).
				Count(&cnt).Error
			waitRank = int(cnt)
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/my_qr.tmpl")
		_ = view.ExecuteTemplate(w, "parents/my_qr.tmpl", map[string]any{
			"Title":        "My Registration • QR",
			"Parent":       parent,
			"Phone":        phone,
			"ChildName":    child.Name,
			"ClassName":    class.Name,
			"DateStr":      fmtDate(class.Date),
			"Status":       reg.Status,
			"WaitlistRank": waitRank,
			"Code":         reg.Code,                // used only when confirmed
			"QRURL":        "/qr/" + reg.Code + ".png", // used only when confirmed
		})
	}
}

