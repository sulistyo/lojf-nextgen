package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

func AdminEditClassForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		var class models.Class
		if err := db.Conn().First(&class, id).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/classes_edit.tmpl")
		_ = view.ExecuteTemplate(w, "admin/classes_edit.tmpl", map[string]any{
			"Title":   "Admin • Edit Class",
			"Class":   class,
			"DateVal": class.Date.Format("2006-01-02"),
			"TimeVal": class.Date.Format("15:04"),
		})
	}
}

func AdminUpdateClass(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	date := r.FormValue("date")
	timeStr := r.FormValue("time")
	name := r.FormValue("name")
	capStr := r.FormValue("capacity")

	var class models.Class
	if err := db.Conn().First(&class, id).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	dtStr := date
	if timeStr != "" {
		dtStr = date + " " + timeStr
	}
	dt, err := time.Parse("2006-01-02 15:04", dtStr)
	if err != nil {
		dt, err = time.Parse("2006-01-02", date)
		if err != nil {
			http.Error(w, "invalid date/time", http.StatusBadRequest)
			return
		}
	}
	capacity, err := strconv.Atoi(capStr)
	if err != nil || capacity < 0 {
		http.Error(w, "invalid capacity", http.StatusBadRequest)
		return
	}

	class.Name = name
	class.Date = dt
	class.Capacity = capacity
	if err := db.Conn().Save(&class).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Re-balance registrations for this class
	_ = svc.RecomputeClass(uint(class.ID))

	http.Redirect(w, r, "/admin/classes", http.StatusSeeOther)
}
