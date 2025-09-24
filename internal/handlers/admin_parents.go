package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type parentRow struct {
	ID           uint
	Name         string
	Phone        string
	Children     int64
	UpcomingRegs int64
}

func AdminParentsList(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		// Load parents
		var parents []models.Parent
		dbq := db.Conn().Order("name asc")
		if q != "" {
			dbq = dbq.Where("name LIKE ? OR phone LIKE ?", "%"+q+"%", "%"+q+"%")
		}
		if err := dbq.Find(&parents).Error; err != nil {
			http.Error(w, "db error", 500); return
		}

		rows := make([]parentRow, 0, len(parents))
		now := time.Now().Add(-2 * time.Hour)
		for _, p := range parents {
			var kids int64
			db.Conn().Model(&models.Child{}).Where("parent_id = ?", p.ID).Count(&kids)
			var upcoming int64
			db.Conn().Table("registrations").
				Joins("JOIN classes ON classes.id = registrations.class_id").
				Where("registrations.parent_id = ? AND classes.date >= ?", p.ID, now).
				Count(&upcoming)
			rows = append(rows, parentRow{
				ID: p.ID, Name: p.Name, Phone: p.Phone,
				Children: kids, UpcomingRegs: upcoming,
			})
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/parents.tmpl")
		_ = view.ExecuteTemplate(w, "admin/parents.tmpl", map[string]any{
			"Title": "Admin • Parents",
			"Rows":  rows,
			"Q":     q,
		})
	}
}

func AdminParentShowForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		var parent models.Parent
		if err := db.Conn().First(&parent, id).Error; err != nil { http.NotFound(w, r); return }
		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		msg := ""
		switch r.URL.Query().Get("ok") {
		case "saved":
			msg = "Parent saved."
		case "child_saved":
			msg = "Child saved."
		case "child_deleted":
			msg = "Child deleted."
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/parent_show.tmpl")
		_ = view.ExecuteTemplate(w, "admin/parent_show.tmpl", map[string]any{
			"Title":  "Admin • Parent",
			"Parent": parent,
			"Kids":   kids,
			"Msg":    msg,
		})
	}
}


func AdminParentUpdate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	name := r.FormValue("name")
	phone := r.FormValue("phone")
	if name == "" || phone == "" { http.Error(w, "missing fields", 400); return }

	var parent models.Parent
	if err := db.Conn().First(&parent, id).Error; err != nil {
		http.NotFound(w, r); return
	}

	// Enforce phone uniqueness (simple check)
	var count int64
	db.Conn().Model(&models.Parent{}).
		Where("phone = ? AND id <> ?", phone, parent.ID).Count(&count)
	if count > 0 { http.Error(w, "phone already in use", 400); return }

	parent.Name = name
	parent.Phone = phone
	if err := db.Conn().Save(&parent).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(int(parent.ID))+"?ok=saved", http.StatusSeeOther)

}

// Admin child edit/delete reuse parent-side forms? For clarity, keep simple admin edit inline.
func AdminChildUpdate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	parentID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	childID, _ := strconv.Atoi(r.FormValue("child_id"))
	name := r.FormValue("child_name")
	dob := r.FormValue("child_dob")

	if childID == 0 || name == "" {
		http.Error(w, "missing child_id or name", 400)
		return
	}

	var child models.Child
	if err := db.Conn().First(&child, childID).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	child.Name = name
	if dob != "" {
		if d, err := time.Parse("2006-01-02", dob); err == nil {
			child.BirthDate = d
		} else {
			http.Error(w, "invalid date", 400)
			return
		}
	}
	if err := db.Conn().Save(&child).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(parentID)+"?ok=child_saved", http.StatusSeeOther)

}

func AdminChildDelete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	parentID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	childID, _ := strconv.Atoi(r.FormValue("child_id"))
	if childID == 0 { http.Error(w, "missing child_id", 400); return }
	var child models.Child
	if err := db.Conn().First(&child, childID).Error; err != nil {
		http.NotFound(w, r); return
	}
	if err := db.Conn().Delete(&child).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(parentID)+"?ok=child_deleted", http.StatusSeeOther)
}
