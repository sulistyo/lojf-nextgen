package handlers

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"time"
	"strings"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func init() { rand.Seed(time.Now().UnixNano()) }

// ------------------- STEP 1: phone entry -------------------
func RegisterPhoneForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If we already know the parent, skip the phone gate
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			http.Redirect(w, r, "/register/kids?phone="+cPhone, http.StatusSeeOther)
			return
		}

		// Optional prefill via ?phone=
		phone := strings.TrimSpace(r.URL.Query().Get("phone"))

		var parent *models.Parent
		if phone != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil {
				parent = &p
				// refresh cookies so subsequent steps remember it
				setParentCookies(w, p.Phone, p.Name)
			}
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/register_phone.tmpl")
		_ = view.ExecuteTemplate(w, "parents/register_phone.tmpl", map[string]any{
			"Title":  "Register • Phone",
			"Phone":  phone,
			"Parent": parent,
		})
	}
}


func RegisterPhoneSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := strings.TrimSpace(r.FormValue("phone"))
	if phone == "" { http.Error(w, "phone required", 400); return }

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err == nil && parent.ID > 0 {
		// Returning parent
		setParentCookies(w, parent.Phone, parent.Name)
		http.Redirect(w, r, "/register/kids?phone="+phone, http.StatusSeeOther)
		return
	}
	// First time (no name yet)
	setParentCookies(w, phone, "")
	http.Redirect(w, r, "/register/onboard?phone="+phone, http.StatusSeeOther)
}

// ------------------- STEP 2a: first-time onboard -------------------
func RegisterOnboardForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" { http.Error(w, "missing phone", 400); return }
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/onboard.tmpl")
		_ = view.ExecuteTemplate(w, "parents/onboard.tmpl", map[string]any{
			"Title": "Register • Details",
			"Phone": phone,
		})
	}
}

func RegisterOnboardSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	parentName := r.FormValue("parent_name")
	childName := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	if phone == "" || parentName == "" || childName == "" || dob == "" {
		http.Error(w, "missing fields", 400)
		return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil {
		http.Error(w, "invalid date", 400)
		return
	}
	// upsert parent by phone
	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err == nil && parent.ID > 0 {
		if parent.Name != parentName {
			parent.Name = parentName
			_ = db.Conn().Save(&parent).Error
		}
	} else {
		parent = models.Parent{Name: parentName, Phone: phone}
		if err := db.Conn().Create(&parent).Error; err != nil {
			http.Error(w, "save parent failed", 500); return
		}
	}
	child := models.Child{Name: childName, BirthDate: d, ParentID: parent.ID}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "save child failed", 500); return
	}
	setParentCookies(w, parent.Phone, parent.Name)
	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", child.ID), http.StatusSeeOther)
}

// ------------------- STEP 2b: returning - choose child -------------------
func RegisterKidsForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" { http.Error(w, "missing phone", 400); return }

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404); return
		}
		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error
		setParentCookies(w, parent.Phone, parent.Name)
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/kids.tmpl")
		_ = view.ExecuteTemplate(w, "parents/kids.tmpl", map[string]any{
			"Title":  "Welcome back",
			"Parent": parent,
			"Kids":   kids,
			"Phone":  phone,
		})
	}
}

func RegisterKidsSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	childSel := r.FormValue("child_id")
	if phone == "" { http.Error(w, "missing phone", 400); return }
	if childSel == "" {
		http.Error(w, "please choose a child or add new", 400); return
	}
	if childSel == "new" {
		http.Redirect(w, r, "/register/newchild?phone="+phone, http.StatusSeeOther)
		return
	}
	childID, _ := strconv.Atoi(childSel)
	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", childID), http.StatusSeeOther)
}

// ------------------- STEP 2c: add a new child -------------------
func RegisterNewChildForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" { http.Error(w, "missing phone", 400); return }

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404); return
		}
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/new_child.tmpl")
		_ = view.ExecuteTemplate(w, "parents/new_child.tmpl", map[string]any{
			"Title":  "Add Child",
			"Parent": parent,
			"Phone":  phone,
		})
	}
}

func RegisterNewChildSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := r.FormValue("phone")
	childName := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	if phone == "" || childName == "" || dob == "" {
		http.Error(w, "missing fields", 400); return
	}
	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", 404); return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil { http.Error(w, "invalid date", 400); return }

	child := models.Child{Name: childName, BirthDate: d, ParentID: parent.ID}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "save child failed", 500); return
	}
	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", child.ID), http.StatusSeeOther)
}

// ------------------- STEP 3: class selection (unchanged logic) -------------------
func SelectClassForm(t *template.Template) http.HandlerFunc {
	type classOption struct {
		ID         uint
		Name       string
		DateStr    string
		Capacity   int
		Confirmed  int
		Waitlisted int
		Left       int
		IsFull     bool
	}
	return func(w http.ResponseWriter, r *http.Request) {
		childIDStr := r.URL.Query().Get("child_id")
		if childIDStr == "" { http.Error(w, "missing child_id", 400); return }
		childID, err := strconv.Atoi(childIDStr); if err != nil { http.Error(w, "invalid child_id", 400); return }

		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil { http.Error(w, "child not found", 404); return }

		var parent models.Parent
		_ = db.Conn().First(&parent, child.ParentID).Error

		now := time.Now()
		from := now.AddDate(0, 0, -90)
		to := now.AddDate(0, 6, 0)

		var classes []models.Class
		_ = db.Conn().
			Model(&models.Class{}).
			Where("classes.date BETWEEN ? AND ?", from, to).
			Order("classes.date asc").
			Find(&classes).Error

		opts := make([]classOption, 0, len(classes))
		for _, c := range classes {
			var confirmed int64
			_ = db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ?", c.ID, "confirmed").
				Count(&confirmed).Error
			var waitlisted int64
			_ = db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ?", c.ID, "waitlisted").
				Count(&waitlisted).Error

			left := c.Capacity - int(confirmed)
			if left < 0 { left = 0 }

			opts = append(opts, classOption{
				ID:         c.ID,
				Name:       c.Name,
				DateStr:    c.Date.Format("Mon, 02 Jan 2006 15:04"),
				Capacity:   c.Capacity,
				Confirmed:  int(confirmed),
				Waitlisted: int(waitlisted),
				Left:       left,
				IsFull:     left == 0,
			})
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/select_class.tmpl")
		_ = view.ExecuteTemplate(w, "parents/select_class.tmpl", map[string]any{
			"Title":"Select Class",
			"Child":child,
			"Parent":parent,
			"Phone": parent.Phone,
			"ClassOptions":opts,
		})
	}
}

func SelectClassSubmit(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		childID, _ := strconv.Atoi(r.FormValue("child_id"))
		classID, _ := strconv.Atoi(r.FormValue("class_id"))

		if childID == 0 {
			http.Error(w, "missing child_id", http.StatusBadRequest); return
		}
		if classID == 0 {
			http.Error(w, "no class selected", http.StatusBadRequest); return
		}

		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil { http.Error(w, "child not found", 404); return }
		var class models.Class
		if err := db.Conn().First(&class, classID).Error; err != nil { http.Error(w, "class not found", 404); return }

		var confirmedCnt int64
		_ = db.Conn().Model(&models.Registration{}).
			Where("class_id = ? AND status = ?", class.ID, "confirmed").Count(&confirmedCnt).Error
		status := "waitlisted"
		if int(confirmedCnt) < class.Capacity {
			status = "confirmed"
		}

		var code string
		for i := 0; i < 5; i++ {
			code = fmt.Sprintf("REG-%06d", rand.Intn(1000000))
			var exists int64
			_ = db.Conn().Model(&models.Registration{}).Where("code = ?", code).Count(&exists).Error
			if exists == 0 { break }
		}

		reg := models.Registration{
			ParentID: child.ParentID,
			ChildID:  child.ID,
			ClassID:  class.ID,
			Status:   status,
			Code:     code,
		}
		if err := db.Conn().Create(&reg).Error; err != nil {
			http.Error(w, "failed to save registration", 500); return
		}

		var rank int64
		if status == "waitlisted" {
			_ = db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = 'waitlisted' AND (created_at < ? OR (created_at = ? AND id <= ?))",
					class.ID, reg.CreatedAt, reg.CreatedAt, reg.ID).
				Count(&rank).Error
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/registration_done.tmpl")
		_ = view.ExecuteTemplate(w, "parents/registration_done.tmpl", map[string]any{
			"Title":"Registration Result",
			"ChildName":child.Name,
			"ClassName":class.Name,
			"Date":class.Date.Format("Mon, 02 Jan 2006"),
			"Status":status,
			"Code":code,
			"Rank":rank,
		})
	}
}
