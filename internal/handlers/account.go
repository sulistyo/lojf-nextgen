package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

// ---------- Phone gate ----------

// GET /account  (auto-jump to /account/profile if cookie present)
func AccountPhoneForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/account_phone.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		// If cookie already set, skip the phone gate
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			http.Redirect(w, r, "/account/profile", http.StatusSeeOther)
			return
		}

		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		var parent *models.Parent
		if phone != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil {
				parent = &p
				setParentCookies(w, p.Phone, p.Name)
			}
		}

		if err := view.ExecuteTemplate(w, "parents/account_phone.tmpl", map[string]any{
			"Title":  "My Account",
			"Phone":  phone,
			"Parent": parent,
		}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

// ---------- Profile (view / update parent name), children list ----------

func AccountProfileForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/account_profile.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		if phone == "" {
			if cPhone, _ := readParentCookies(r); cPhone != "" {
				phone = cPhone
			}
		}
		if phone == "" {
			http.Error(w, "missing phone", http.StatusBadRequest)
			return
		}

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", http.StatusNotFound)
			return
		}

		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		// Telegram link status
		var tg models.TelegramUser
		linked := false
		if err := db.Conn().Where("parent_id = ? AND deliverable = 1", parent.ID).First(&tg).Error; err == nil {
			linked = true
		}

		_ = view.ExecuteTemplate(w, "parents/account_profile.tmpl", map[string]any{
			"Title":    "My Account",
			"Parent":   parent,
			"Kids":     kids,
			"Phone":    phone,
			"LinkCode": r.URL.Query().Get("link_code"),
			"TGLinked": linked,
			"TG":       tg,
			"Flash":    MakeFlash(r, "", ""), // ← unified flash (query ok/error OR handler messages)
		})
	}
}

// POST /account/profile
func AccountProfileSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	name := strings.TrimSpace(r.FormValue("parent_name"))
	newPhone := svc.NormPhone(r.FormValue("phone"))

	// Optional email (normalized + validated)
	emailRaw := r.FormValue("email")
	email, ok := svc.NormEmail(emailRaw) // returns "",true if empty is OK
	if !ok {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}

	if name == "" || newPhone == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}

	// Prefer cookie (in case phone was changed)
	cookiePhone, _ := readParentCookies(r)

	var parent models.Parent
	var err error
	if cookiePhone != "" {
		err = db.Conn().Where("phone = ?", cookiePhone).First(&parent).Error
	}
	if err != nil {
		// fallback to posted phone
		if e := db.Conn().Where("phone = ?", newPhone).First(&parent).Error; e != nil {
			http.Error(w, "parent not found", http.StatusNotFound)
			return
		}
	}

	parent.Name = name
	parent.Phone = newPhone
	parent.Email = email // empty string = "unset"

	if err := db.Conn().Save(&parent).Error; err != nil {
		le := strings.ToLower(err.Error())
		if strings.Contains(le, "unique") && strings.Contains(le, "email") {
			http.Redirect(w, r, "/account/profile?error=email_in_use", http.StatusSeeOther)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	setParentCookies(w, parent.Phone, parent.Name)
	http.Redirect(w, r, "/account/profile?ok=saved", http.StatusSeeOther)
}

// ---------- Add/Edit/Delete child ----------

func AccountNewChildForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/account_child_new.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		if phone == "" {
			http.Error(w, "missing phone", 400)
			return
		}

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404)
			return
		}

		if err := view.ExecuteTemplate(w, "parents/account_child_new.tmpl", map[string]any{
			"Title":  "Add Child",
			"Phone":  phone,
			"Parent": parent,
		}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

func AccountNewChildSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := svc.NormPhone(r.FormValue("phone"))
	name := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	if phone == "" || name == "" || dob == "" {
		http.Error(w, "missing fields", 400)
		return
	}

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", 404)
		return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil {
		http.Error(w, "invalid date", 400)
		return
	}

	child := models.Child{Name: name, BirthDate: d, ParentID: parent.ID}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	http.Redirect(w, r, "/account/profile?ok=child_saved", http.StatusSeeOther)

}

func AccountEditChildForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/account_child_edit.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		phone := r.URL.Query().Get("phone")
		id, _ := strconv.Atoi(idStr)
		var child models.Child
		if err := db.Conn().First(&child, id).Error; err != nil {
			http.Error(w, "child not found", 404)
			return
		}
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

	phone := svc.NormPhone(r.FormValue("phone"))
	if phone == "" {
		// fallback to cookie if the form didn’t include phone
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			phone = svc.NormPhone(cPhone)
		}
	}

	childID, _ := strconv.Atoi(r.FormValue("child_id"))
	name := strings.TrimSpace(r.FormValue("child_name"))
	dob := strings.TrimSpace(r.FormValue("child_dob"))
	gender := normGender(r.FormValue("child_gender"))

	if phone == "" || childID == 0 || name == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", http.StatusNotFound)
		return
	}
	var child models.Child
	if err := db.Conn().First(&child, childID).Error; err != nil || child.ParentID != parent.ID {
		http.Error(w, "child not found", http.StatusNotFound)
		return
	}

	child.Name = name
	if dob != "" {
		if d, err := time.Parse("2006-01-02", dob); err == nil {
			child.BirthDate = d
		} else {
			http.Error(w, "invalid date", http.StatusBadRequest)
			return
		}
	}
	child.Gender = gender

	if err := db.Conn().Save(&child).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account/profile?ok=child_saved", http.StatusSeeOther)
}



func AccountDeleteChild(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := svc.NormPhone(r.FormValue("phone"))
	idStr := r.FormValue("id")
	id, _ := strconv.Atoi(idStr)
	if phone == "" || id == 0 {
		http.Error(w, "missing fields", 400)
		return
	}

	var child models.Child
	if err := db.Conn().First(&child, id).Error; err != nil {
		http.Error(w, "child not found", 404)
		return
	}
	// NOTE: You may want to block delete if child has future registrations.
	if err := db.Conn().Delete(&child).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	http.Redirect(w, r, "/account/profile?ok=child_deleted", http.StatusSeeOther)

}
