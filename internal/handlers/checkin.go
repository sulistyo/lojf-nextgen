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

// GET /checkin
func CheckinForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(r.URL.Query().Get("code"))

		var row *checkinRow
		errMsg := ""

		if code != "" {
			var rr checkinRow
			if err := db.Conn().Table("registrations r").
				Select(`r.code, r.status, r.check_in_at,
						children.name as child_name,
						classes.name  as class_name,
						classes.date  as class_date`).
				Joins("JOIN children ON children.id = r.child_id").
				Joins("JOIN classes  ON classes.id = r.class_id").
				Where("r.code = ?", code).
				Scan(&rr).Error; err == nil && rr.Code != "" {

				loc, _ := time.LoadLocation("Asia/Jakarta")
				rr.DateStr = rr.ClassDate.In(loc).Format("Mon, 02 Jan 2006 15:04")

				// Optional hint on GET: if not eligible, surface a friendly message
				if rr.Status != "confirmed" {
					errMsg = "Only CONFIRMED registrations can be checked in."
				}
				row = &rr
			}
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/checkin.tmpl")
		_ = view.ExecuteTemplate(w, "admin/checkin.tmpl", checkinVM{
			Title: "Admin • Check-in",
			Code:  code,
			Reg:   row,
			Flash: MakeFlash(r, errMsg, ""), // unified flash
		})
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
