package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func AdminClasses(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var classes []models.Class
		if err := db.Conn().Order("date asc").Find(&classes).Error; err != nil {
			http.Error(w, "db error", 500); return
		}
		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/classes.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		data := map[string]any{"Title": "Admin • Classes", "Classes": classes}
		if err := view.ExecuteTemplate(w, "admin/classes.tmpl", data); err != nil {
			http.Error(w, err.Error(), 500); return
		}
	}
}

func AdminNewClass(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/classes_new.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		data := map[string]any{"Title": "Admin • New Class"}
		if err := view.ExecuteTemplate(w, "admin/classes_new.tmpl", data); err != nil {
			http.Error(w, err.Error(), 500); return
		}
	}
}

func AdminCreateClass(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil { http.Error(w, err.Error(), 400); return }
	dateStr := r.FormValue("date")
	name := r.FormValue("name")
	capStr := r.FormValue("capacity")
	if dateStr=="" || name=="" || capStr=="" { http.Error(w,"missing fields",400); return }

	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil { http.Error(w,"invalid date",400); return }
	capacity, err := strconv.Atoi(capStr)
	if err != nil || capacity < 0 { http.Error(w,"invalid capacity",400); return }

	cl := models.Class{Date: d, Name: name, Capacity: capacity}
	if err := db.Conn().Create(&cl).Error; err != nil { http.Error(w,"db error",500); return }
	http.Redirect(w, r, "/admin/classes", http.StatusSeeOther)
}
