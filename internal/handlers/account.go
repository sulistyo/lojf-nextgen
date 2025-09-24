package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"time"
	"strings"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// ---------- Phone gate ----------

// GET /account  (auto-jump to /account/profile if cookie present)
func AccountPhoneForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If cookie already set, skip the phone gate
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			http.Redirect(w, r, "/account/profile", http.StatusSeeOther)
			return
		}

		phone := r.URL.Query().Get("phone")
		var parent *models.Parent
		if phone != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil {
				parent = &p
				setParentCookies(w, p.Phone, p.Name)
			}
		}

		view, err := t.Clone(); if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/account_phone.tmpl"); err != nil { http.Error(w, err.Error(), 500); return }
		if err := view.ExecuteTemplate(w, "parents/account_phone.tmpl", map[string]any{
			"Title":  "My Account",
			"Phone":  phone,
			"Parent": parent,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}


// ---------- Profile (view / update parent name), children list ----------

func AccountProfileForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" {
			if cPhone, _ := readParentCookies(r); cPhone != "" {
				phone = cPhone
			}
		}
		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404); return
		}
		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		msg := ""
		switch r.URL.Query().Get("ok") {
		case "saved":
			msg = "Profile saved."
		case "child_saved":
			msg = "Child saved."
		case "child_deleted":
			msg = "Child deleted."
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/account_profile.tmpl")
		_ = view.ExecuteTemplate(w, "parents/account_profile.tmpl", map[string]any{
			"Title":  "My Account",
			"Parent": parent,
			"Kids":   kids,
			"Phone":  phone,
			"Msg":    msg,
		})
	}
}

func AccountProfileSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	name := r.FormValue("parent_name")
	if phone == "" || name == "" { http.Error(w, "missing fields", 400); return }

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", 404); return
	}
	parent.Name = name
	if err := db.Conn().Save(&parent).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	setParentCookies(w, parent.Phone, parent.Name)
	http.Redirect(w, r, "/account/profile?ok=saved", http.StatusSeeOther)
}

// ---------- Add/Edit/Delete child ----------

func AccountNewChildForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" { http.Error(w, "missing phone", 400); return }

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404); return
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/account_child_new.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		if err := view.ExecuteTemplate(w, "parents/account_child_new.tmpl", map[string]any{
			"Title":  "Add Child",
			"Phone":  phone,
			"Parent": parent,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}

func AccountNewChildSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	name := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	if phone == "" || name == "" || dob == "" { http.Error(w, "missing fields", 400); return }

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", 404); return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil { http.Error(w, "invalid date", 400); return }

	child := models.Child{Name: name, BirthDate: d, ParentID: parent.ID}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	http.Redirect(w, r, "/account/profile?ok=child_saved", http.StatusSeeOther)

}

func AccountEditChildForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		phone := r.URL.Query().Get("phone")
		id, _ := strconv.Atoi(idStr)
		var child models.Child
		if err := db.Conn().First(&child, id).Error; err != nil {
			http.Error(w, "child not found", 404); return
		}
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/account_child_edit.tmpl")
		_ = view.ExecuteTemplate(w, "parents/account_child_edit.tmpl", map[string]any{
			"Title":     "Edit Child",
			"Child":     child,
			"Phone":     phone,
			"BirthDate": child.BirthDate.Format("2006-01-02"),
		})
	}
}

func AccountEditChildSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	idStr := r.FormValue("id")
	name := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	if phone == "" || idStr == "" || name == "" || dob == "" { http.Error(w, "missing fields", 400); return }
	id, _ := strconv.Atoi(idStr)

	var child models.Child
	if err := db.Conn().First(&child, id).Error; err != nil {
		http.Error(w, "child not found", 404); return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil { http.Error(w, "invalid date", 400); return }
	child.Name = name
	child.BirthDate = d
	if err := db.Conn().Save(&child).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	http.Redirect(w, r, "/account/profile?ok=child_saved", http.StatusSeeOther)

}

func AccountDeleteChild(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	idStr := r.FormValue("id")
	id, _ := strconv.Atoi(idStr)
	if phone == "" || id == 0 { http.Error(w, "missing fields", 400); return }

	var child models.Child
	if err := db.Conn().First(&child, id).Error; err != nil {
		http.Error(w, "child not found", 404); return
	}
	// NOTE: You may want to block delete if child has future registrations.
	if err := db.Conn().Delete(&child).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	http.Redirect(w, r, "/account/profile?ok=child_deleted", http.StatusSeeOther)

}
