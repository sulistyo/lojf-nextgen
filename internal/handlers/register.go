package handlers

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"strings"
	svc "github.com/lojf/nextgen/internal/services"
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
		phone := svc.NormPhone(r.URL.Query().Get("phone"))

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
	phone := svc.NormPhone(r.FormValue("phone"))
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

// POST /register/onboard
func RegisterOnboardSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	phone := svc.NormPhone(r.FormValue("phone"))
	parentName := strings.TrimSpace(r.FormValue("parent_name"))
	childName := strings.TrimSpace(r.FormValue("child_name"))
	dob := strings.TrimSpace(r.FormValue("child_dob"))

	// Optional email
	emailRaw := r.FormValue("email")
	email, ok := svc.NormEmail(emailRaw) // "" is allowed
	if !ok {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}

	if phone == "" || parentName == "" || childName == "" || dob == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}

	d, err := time.Parse("2006-01-02", dob)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	// Upsert parent by normalized phone
	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err == nil && parent.ID > 0 {
		changed := false
		if parent.Name != parentName {
			parent.Name = parentName
			changed = true
		}
		if parent.Phone != phone {
			parent.Phone = phone
			changed = true
		}
		if parent.Email != email { // email may be ""
			parent.Email = email
			changed = true
		}
		if changed {
			if err := db.Conn().Save(&parent).Error; err != nil {
				le := strings.ToLower(err.Error())
				if strings.Contains(le, "unique") && strings.Contains(le, "email") {
					http.Error(w, "email already used by another account", http.StatusConflict)
					return
				}
				http.Error(w, "save parent failed", http.StatusInternalServerError)
				return
			}
		}
	} else {
		parent = models.Parent{Name: parentName, Phone: phone, Email: email}
		if err := db.Conn().Create(&parent).Error; err != nil {
			le := strings.ToLower(err.Error())
			if strings.Contains(le, "unique") && strings.Contains(le, "email") {
				http.Error(w, "email already used by another account", http.StatusConflict)
				return
			}
			http.Error(w, "save parent failed", http.StatusInternalServerError)
			return
		}
	}

	// Create child
	child := models.Child{Name: childName, BirthDate: d, ParentID: parent.ID}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "save child failed", http.StatusInternalServerError)
		return
	}

	setParentCookies(w, parent.Phone, parent.Name)
	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", child.ID), http.StatusSeeOther)
}


// ------------------- STEP 2b: returning - choose child -------------------
// RegisterKidsForm shows the children list for the parent (phone from query or cookie)
func RegisterKidsForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		if strings.TrimSpace(phone) == "" {
			if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
				phone = cPhone
			}
		}
		// tolerant parent lookup
		p, err := svc.FindParentByAny(phone)
		if err != nil {
			http.Error(w, "parent not found", http.StatusNotFound)
			return
		}
		parent := *p

		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		// keep cookies fresh and normalized
		setParentCookies(w, parent.Phone, parent.Name)

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/kids.tmpl")
		_ = view.ExecuteTemplate(w, "parents/kids.tmpl", map[string]any{
			"Title":  "Welcome back",
			"Parent": parent,
			"Kids":   kids,
			"Phone":  parent.Phone, // normalized phone
		})
	}
}


// RegisterKidsSubmit handles child selection or "add new child"
func RegisterKidsSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := svc.NormPhone(r.FormValue("phone"))
	if strings.TrimSpace(phone) == "" {
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			phone = cPhone
		}
	}
	if strings.TrimSpace(phone) == "" {
		http.Error(w, "missing phone", http.StatusBadRequest)
		return
	}

	childSel := r.FormValue("child_id")
	if childSel == "" {
		http.Error(w, "please choose a child or add new", http.StatusBadRequest)
		return
	}

	if childSel == "new" {
		http.Redirect(w, r, "/register/newchild?phone="+url.QueryEscape(phone), http.StatusSeeOther)
		return
	}

	childID, err := strconv.Atoi(childSel)
	if err != nil || childID <= 0 {
		http.Error(w, "invalid child", http.StatusBadRequest)
		return
	}

	// (Optional safety) ensure the child belongs to this parent
	var cnt int64
	db.Conn().Model(&models.Child{}).Where("id = ? AND parent_id = (SELECT id FROM parents WHERE phone = ?)", childID, phone).Count(&cnt)
	if cnt == 0 {
		http.Error(w, "child not found for this parent", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", childID), http.StatusSeeOther)
}


// ------------------- STEP 2c: add a new child -------------------
func RegisterNewChildForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
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

	type classRow struct {
		ID         uint
		Name       string
		Date       time.Time
		Capacity   int
		Confirmed  int64
		Waitlisted int64
	}

	return func(w http.ResponseWriter, r *http.Request) {
		childIDStr := r.URL.Query().Get("child_id")
		if childIDStr == "" {
			http.Error(w, "missing child_id", http.StatusBadRequest)
			return
		}
		childID, err := strconv.Atoi(childIDStr)
		if err != nil || childID <= 0 {
			http.Error(w, "invalid child_id", http.StatusBadRequest)
			return
		}

		// Load child & parent
		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound)
			return
		}
		var parent models.Parent
		_ = db.Conn().First(&parent, child.ParentID).Error

		// Time window: from "now" (UTC) to +6 months
		nowUTC := time.Now().UTC()
		toUTC := nowUTC.AddDate(0, 6, 0)

		// Aggregate classes with counts in one query
		var rows []classRow
		if err := db.Conn().
			Table("classes c").
			Select(`
				c.id, c.name, c.date, c.capacity,
				COALESCE(SUM(CASE WHEN r.status = 'confirmed'  THEN 1 ELSE 0 END), 0) AS confirmed,
				COALESCE(SUM(CASE WHEN r.status = 'waitlisted' THEN 1 ELSE 0 END), 0) AS waitlisted
			`).
			Joins(`LEFT JOIN registrations r ON r.class_id = c.id AND r.status IN ('confirmed','waitlisted')`).
			Where("c.date BETWEEN ? AND ?", nowUTC, toUTC).
			Group("c.id").
			Order("c.date ASC").
			Scan(&rows).Error; err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		// Build options
		opts := make([]classOption, 0, len(rows))
		for _, rrow := range rows {
			left := rrow.Capacity - int(rrow.Confirmed)
			if left < 0 {
				left = 0
			}
			opts = append(opts, classOption{
				ID:         rrow.ID,
				Name:       rrow.Name,
				DateStr:    fmtDate(rrow.Date), // your helper already handles Asia/Jakarta
				Capacity:   rrow.Capacity,
				Confirmed:  int(rrow.Confirmed),
				Waitlisted: int(rrow.Waitlisted),
				Left:       left,
				IsFull:     left == 0,
			})
		}

		// Render
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/select_class.tmpl")
		_ = view.ExecuteTemplate(w, "parents/select_class.tmpl", map[string]any{
			"Title":        "Select Class",
			"Child":        child,
			"Parent":       parent,
			"Phone":        parent.Phone,
			"ClassOptions": opts,
			"Flash":        MakeFlash(r, "", ""), // use unified flash; remove `.Err` in template
		})
	}
}


func SelectClassSubmit(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		childID, _ := strconv.Atoi(r.FormValue("child_id"))
		classID, _ := strconv.Atoi(r.FormValue("class_id"))
		if childID <= 0 {
			http.Error(w, "missing child_id", http.StatusBadRequest); return
		}
		if classID <= 0 {
			http.Error(w, "no class selected", http.StatusBadRequest); return
		}

		// Validate conflicts (duplicate class / same-day)
		if err := svc.CheckRegistrationConflicts(uint(childID), uint(classID)); err != nil {
			switch err {
			case svc.ErrDuplicateReg:
				http.Redirect(w, r,
					"/register/classes?child_id="+strconv.Itoa(childID)+"&error=already_registered",
					http.StatusSeeOther)
				return
			case svc.ErrSameDayReg:
				http.Redirect(w, r,
					"/register/classes?child_id="+strconv.Itoa(childID)+"&error=same_day_conflict",
					http.StatusSeeOther)
				return
			default:
				http.Error(w, "validation error", http.StatusBadRequest)
				return
			}
		}

		// Load child & class
		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound); return
		}
		var class models.Class
		if err := db.Conn().First(&class, classID).Error; err != nil {
			http.Error(w, "class not found", http.StatusNotFound); return
		}

		// Provisional status based on current confirmed count
		var confirmedCnt int64
		_ = db.Conn().Model(&models.Registration{}).
			Where("class_id = ? AND status = ?", class.ID, "confirmed").
			Count(&confirmedCnt).Error
		status := "waitlisted"
		if int(confirmedCnt) < class.Capacity {
			status = "confirmed"
		}

		// Generate unique registration code
		var code string
		for i := 0; i < 10; i++ {
			code = fmt.Sprintf("REG-%06d", rand.Intn(1000000))
			var exists int64
			_ = db.Conn().Model(&models.Registration{}).Where("code = ?", code).Count(&exists).Error
			if exists == 0 {
				break
			}
		}
		if code == "" {
			http.Error(w, "failed to generate code", http.StatusInternalServerError); return
		}

		// Create registration
		reg := models.Registration{
			ParentID: child.ParentID,
			ChildID:  child.ID,
			ClassID:  class.ID,
			Status:   status,
			Code:     code,
		}
		if err := db.Conn().Create(&reg).Error; err != nil {
			http.Error(w, "failed to save registration", http.StatusInternalServerError); return
		}

		// Rebalance class (authoritative capacity enforcement),
		// then reload to get the final status.
		_ = svc.RecomputeClass(uint(class.ID))
		_ = db.Conn().First(&reg, reg.ID).Error
		status = reg.Status

		// Compute waitlist rank (FIFO) if still waitlisted
		var rank int64
		if status == "waitlisted" {
			_ = db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = 'waitlisted' AND (created_at < ? OR (created_at = ? AND id <= ?))",
					class.ID, reg.CreatedAt, reg.CreatedAt, reg.ID).
				Count(&rank).Error
		}

		// Render result page
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/registration_done.tmpl")
		_ = view.ExecuteTemplate(w, "parents/registration_done.tmpl", map[string]any{
			"Title":     "Registration Result",
			"ChildName": child.Name,
			"ClassName": class.Name,
			"Date":      fmtDate(class.Date), // uses your Asia/Jakarta formatting helper
			"Status":    status,
			"Code":      code,
			"Rank":      rank,
		})
	}
}

