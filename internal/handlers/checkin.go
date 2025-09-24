package handlers

import (
	"html/template"
	"net/http"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type checkinView struct {
	Title        string
	Code         string
	Found        bool
	AlreadyIn    bool
	ChildName    string
	ClassName    string
	DateStr      string
	Status       string
	ErrorMessage string
}

func CheckinForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/checkin.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}

		code := r.URL.Query().Get("code")
		vm := checkinView{Title: "Admin • Check-in"}
		if code == "" {
			_ = view.ExecuteTemplate(w, "admin/checkin.tmpl", vm)
			return
		}
		vm.Code = code

		var reg models.Registration
		if err := db.Conn().Where("code = ?", code).First(&reg).Error; err != nil {
			vm.ErrorMessage = "Code not found."
			_ = view.ExecuteTemplate(w, "admin/checkin.tmpl", vm)
			return
		}
		vm.Found = true
		vm.Status = reg.Status
		if reg.CheckInAt != nil {
			vm.AlreadyIn = true
		}
		var child models.Child
		_ = db.Conn().First(&child, reg.ChildID).Error
		var class models.Class
		_ = db.Conn().First(&class, reg.ClassID).Error
		vm.ChildName = child.Name
		vm.ClassName = class.Name
		vm.DateStr = class.Date.Format("Mon, 02 Jan 2006 15:04")

		// Block non-confirmed
		if reg.Status != "confirmed" {
			vm.ErrorMessage = "Only CONFIRMED registrations can be checked in."
		}

		_ = view.ExecuteTemplate(w, "admin/checkin.tmpl", vm)
	}
}

func CheckinConfirm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil { http.Error(w, err.Error(), 400); return }
		code := r.FormValue("code")
		if code == "" { http.Error(w, "missing code", 400); return }

		var reg models.Registration
		if err := db.Conn().Where("code = ?", code).First(&reg).Error; err != nil {
			http.Error(w, "code not found", 404); return
		}
		if reg.Status != "confirmed" {
			http.Error(w, "only CONFIRMED registrations can be checked in", 400)
			return
		}
		now := time.Now()
		if reg.CheckInAt == nil {
			reg.CheckInAt = &now
			if err := db.Conn().Save(&reg).Error; err != nil {
				http.Error(w, "db error", 500); return
			}
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/checkin_done.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		_ = view.ExecuteTemplate(w, "admin/checkin_done.tmpl", map[string]any{
			"Title": "Checked-in",
			"Code":  code,
		})
	}
}
