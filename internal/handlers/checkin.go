package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type checkinRow struct {
	Code      string
	Status    string
	ChildName string
	ClassName string
	ClassDate time.Time
	CheckInAt *time.Time
	DateStr   string
}

type checkinVM struct {
	Title string
	Code  string
	Reg   *checkinRow
	Flash *Flash
}

func CheckinForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(r.URL.Query().Get("code"))

		var row *checkinRow
		errMsg := ""

		if code != "" {
			var reg models.Registration
			if err := db.Conn().Where("code = ?", code).First(&reg).Error; err == nil && reg.ID != 0 {
				var child models.Child
				_ = db.Conn().First(&child, reg.ChildID).Error
				var class models.Class
				_ = db.Conn().First(&class, reg.ClassID).Error

				rr := checkinRow{
					Code:      reg.Code,
					Status:    reg.Status,
					ChildName: child.Name,
					ClassName: class.Name,
					ClassDate: class.Date,
					CheckInAt: reg.CheckInAt,
				}
				loc, _ := time.LoadLocation("Asia/Jakarta")
				rr.DateStr = rr.ClassDate.In(loc).Format("Mon, 02 Jan 2006 15:04")

				if rr.Status != "confirmed" {
					errMsg = "Only CONFIRMED registrations can be checked in."
				}
				row = &rr
			}
		}

		view, err := t.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := view.ParseFiles("templates/pages/admin/checkin.tmpl"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := view.ExecuteTemplate(w, "admin/checkin.tmpl", checkinVM{
			Title: "Admin • Check-in",
			Code:  code,
			Reg:   row,
			Flash: MakeFlash(r, errMsg, ""),
		}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
}

// POST /checkin
func CheckinConfirm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := strings.TrimSpace(r.FormValue("code"))
		if code == "" {
			http.Redirect(w, r, "/checkin?error=invalid_code", http.StatusSeeOther)
			return
		}

		var reg models.Registration
		if err := db.Conn().Where("code = ?", code).First(&reg).Error; err != nil || reg.ID == 0 {
			http.Redirect(w, r, "/checkin?error=code_not_found", http.StatusSeeOther)
			return
		}
		// Business rules for eligibility
		if reg.Status != "confirmed" {
			http.Redirect(w, r, "/checkin?error=invalid_checkin&code="+code, http.StatusSeeOther)
			return
		}
		if reg.CheckInAt != nil {
			http.Redirect(w, r, "/checkin?error=already_checkedin&code="+code, http.StatusSeeOther)
			return
		}

		now := time.Now()
		reg.CheckInAt = &now
		if err := db.Conn().Save(&reg).Error; err != nil {
			http.Redirect(w, r, "/checkin?error=invalid_checkin&code="+code, http.StatusSeeOther)
			return
		}

		// success → back to GET so the page can show details + green flash
		http.Redirect(w, r, "/checkin?ok=checked_in&code="+code, http.StatusSeeOther)
	}
}
