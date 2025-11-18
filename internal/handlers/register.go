package handlers

import (
	"fmt"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func init() { rand.Seed(time.Now().UnixNano()) }

// ------------------- STEP 1: phone entry -------------------
func RegisterPhoneForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Optional: allow clearing stale cookie: /register?clear=1
		if r.URL.Query().Get("clear") == "1" {
			http.SetCookie(w, &http.Cookie{Name: "parent_phone", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})
			http.SetCookie(w, &http.Cookie{Name: "parent_name", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})
		}

		// If we have a cookie, only trust it if the parent actually exists
		if cPhone, _ := readParentCookies(r); strings.TrimSpace(cPhone) != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", cPhone).First(&p).Error; err == nil && p.ID > 0 {
				// valid cookie → skip phone gate
				http.Redirect(w, r, "/register/kids?phone="+url.QueryEscape(cPhone), http.StatusSeeOther)
				return
			}
			// stale cookie → clear it
			http.SetCookie(w, &http.Cookie{Name: "parent_phone", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})
			http.SetCookie(w, &http.Cookie{Name: "parent_name", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})
		}

		// Optional prefill via ?phone= (do NOT set cookie unless parent exists)
		phone := svc.NormPhone(r.URL.Query().Get("phone"))
		if phone != "" {
			var p models.Parent
			if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil && p.ID > 0 {
				// existing parent supplied via query → set cookie and go straight to kids
				setParentCookies(w, p.Phone, p.Name)
				http.Redirect(w, r, "/register/kids?phone="+url.QueryEscape(phone), http.StatusSeeOther)
				return
			}
		}

		// Render phone entry form (no cookie set yet)
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/register_phone.tmpl")
		_ = view.ExecuteTemplate(w, "parents/register_phone.tmpl", map[string]any{
			"Title": "Register • Phone",
			"Phone": phone, // just a prefill hint
			"Flash": MakeFlash(r, "", ""),
		})
	}
}

func RegisterPhoneSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	phone := svc.NormPhone(r.FormValue("phone"))
	if phone == "" {
		http.Error(w, "phone required", http.StatusBadRequest)
		return
	}

	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err == nil && parent.ID > 0 {
		// Returning parent → set cookies and continue
		setParentCookies(w, parent.Phone, parent.Name)
		http.Redirect(w, r, "/register/kids?phone="+url.QueryEscape(phone), http.StatusSeeOther)
		return
	}

	// New number → DO NOT set cookie yet. Ensure cookies are cleared, then onboard.
	http.SetCookie(w, &http.Cookie{Name: "parent_phone", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})
	http.SetCookie(w, &http.Cookie{Name: "parent_name", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0)})

	http.Redirect(w, r, "/register/onboard?phone="+url.QueryEscape(phone), http.StatusSeeOther)
}

// ------------------- STEP 2a: first-time onboard -------------------
func RegisterOnboardForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		phone := r.URL.Query().Get("phone")
		if phone == "" {
			http.Error(w, "missing phone", 400)
			return
		}
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

	phone      := svc.NormPhone(r.FormValue("phone"))
	parentName := strings.TrimSpace(r.FormValue("parent_name"))
	childName  := strings.TrimSpace(r.FormValue("child_name"))
	dob        := strings.TrimSpace(r.FormValue("child_dob"))
	childGender := strings.TrimSpace(r.FormValue("child_gender")) // NEW (required)

	// Optional email
	emailRaw := r.FormValue("email")
	email, ok := svc.NormEmail(emailRaw) // "" is allowed
	if !ok {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}

	// Basic required checks (gender now required)
	if phone == "" || parentName == "" || childName == "" || dob == "" || childGender == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	// Whitelist gender
	switch childGender {
	case "male", "female": // male, female
	default:
		http.Error(w, "invalid gender", http.StatusBadRequest)
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
		if parent.Name != parentName { parent.Name = parentName; changed = true }
		if parent.Phone != phone     { parent.Phone = phone;     changed = true }
		if parent.Email != email     { parent.Email = email;     changed = true }
		if changed {
			if err := db.Conn().Save(&parent).Error; err != nil {
				le := strings.ToLower(err.Error())
				if strings.Contains(le, "unique") && strings.Contains(le, "email") {
					http.Error(w, "email already used by another account", http.StatusConflict); return
				}
				http.Error(w, "save parent failed", http.StatusInternalServerError); return
			}
		}
	} else {
		parent = models.Parent{Name: parentName, Phone: phone, Email: email}
		if err := db.Conn().Create(&parent).Error; err != nil {
			le := strings.ToLower(err.Error())
			if strings.Contains(le, "unique") && strings.Contains(le, "email") {
				http.Error(w, "email already used by another account", http.StatusConflict); return
			}
			http.Error(w, "save parent failed", http.StatusInternalServerError); return
		}
	}

	// Create first child with Gender (NEW)
	child := models.Child{
		Name:      childName,
		BirthDate: d,
		ParentID:  parent.ID,
		Gender:    childGender, // NEW
	}
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

		// No phone at all → go to phone gate
		if strings.TrimSpace(phone) == "" {
			http.Redirect(w, r, "/register", http.StatusSeeOther)
			return
		}

		// Tolerant parent lookup (by normalized/any)
		p, err := svc.FindParentByAny(phone)
		if err != nil || p == nil {
			// New number: send to onboarding (do NOT set cookies yet)
			http.Redirect(w, r, "/register/onboard?phone="+url.QueryEscape(phone), http.StatusSeeOther)
			return
		}
		parent := *p

		// Safe now to refresh cookies (parent exists)
		setParentCookies(w, parent.Phone, parent.Name)

		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/kids.tmpl")
		_ = view.ExecuteTemplate(w, "parents/kids.tmpl", map[string]any{
			"Title":  "Welcome back",
			"Parent": parent,
			"Kids":   kids,
			"Phone":  parent.Phone,         // normalized phone
			"Flash":  MakeFlash(r, "", ""), // optional: show messages if any
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
		if phone == "" {
			http.Error(w, "missing phone", 400)
			return
		}

		var parent models.Parent
		if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
			http.Error(w, "parent not found", 404)
			return
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
	gender := normGender(r.FormValue("child_gender")) // NEW

	if phone == "" || childName == "" || dob == "" {
		http.Error(w, "missing fields", http.StatusBadRequest); return
	}
	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Error(w, "parent not found", http.StatusNotFound); return
	}
	d, err := time.Parse("2006-01-02", dob)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest); return
	}

	child := models.Child{
		Name:      childName,
		BirthDate: d,
		ParentID:  parent.ID,
		Gender:    gender, // NEW
	}
	if err := db.Conn().Create(&child).Error; err != nil {
		http.Error(w, "save child failed", http.StatusInternalServerError); return
	}
	http.Redirect(w, r, fmt.Sprintf("/register/classes?child_id=%d", child.ID), http.StatusSeeOther)
}

func SelectClassForm(t *template.Template) http.HandlerFunc {
	type classOption struct {
		ID             uint
		Name           string
		DateStr        string
		Capacity       int
		Confirmed      int
		Waitlisted     int
		Left           int
		IsFull         bool
		Description    string
		SignupOpensAt  *time.Time
		OpensAtUnix    int64
		OpensInSeconds int64
		CanRegister    bool
	}
	type classRow struct {
		ID            uint
		Name          string
		Date          time.Time
		Capacity      int
		Confirmed     int64
		Waitlisted    int64
		Description   string
		SignupOpensAt *time.Time
	}

	return func(w http.ResponseWriter, r *http.Request) {
		childIDStr := r.URL.Query().Get("child_id")
		if childIDStr == "" { http.Error(w, "missing child_id", http.StatusBadRequest); return }
		childID, err := strconv.Atoi(childIDStr); if err != nil || childID <= 0 {
			http.Error(w, "invalid child_id", http.StatusBadRequest); return
		}

		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound); return
		}
		var parent models.Parent
		_ = db.Conn().First(&parent, child.ParentID).Error

		errStr := ""
		switch r.URL.Query().Get("error") {
		case "already_registered":
			errStr = "This child is already registered for this class."
		case "same_day_conflict":
			errStr = "This child already has a registration on that day."
		case "not_open_yet":
			errStr = "Registration for this class is not open yet."
		}

		// ---- Time window: include ALL of “today” in Jakarta ----
		locJKT, _ := time.LoadLocation("Asia/Jakarta")
		nowJKT := time.Now().In(locJKT)
		startOfTodayJKT := time.Date(nowJKT.Year(), nowJKT.Month(), nowJKT.Day(), 0, 0, 0, 0, locJKT)
		fromUTC := startOfTodayJKT.UTC()         // start of today's date in Jakarta
		toUTC   := nowJKT.AddDate(0, 6, 0).UTC() // +6 months

		var rows []classRow
		if err := db.Conn().
			Table("classes AS c").
			Select(`
				c.id, c.name, c.date, c.capacity, c.description, c.signup_opens_at,
				COALESCE(SUM(CASE WHEN r.status = 'confirmed'  THEN 1 ELSE 0 END), 0) AS confirmed,
				COALESCE(SUM(CASE WHEN r.status = 'waitlisted' THEN 1 ELSE 0 END), 0) AS waitlisted
			`).
			Joins(`LEFT JOIN registrations r ON r.class_id = c.id AND r.status IN ('confirmed','waitlisted')`).
			Where("c.date BETWEEN ? AND ?", fromUTC, toUTC).
			Group("c.id").
			Order("c.date ASC").
			Scan(&rows).Error; err != nil {
			http.Error(w, "db error", http.StatusInternalServerError); return
		}

		opts := make([]classOption, 0, len(rows))
		for _, rr := range rows {
			left := rr.Capacity - int(rr.Confirmed)
			if left < 0 { left = 0 }

			var opensAtUnix, opensIn int64
			canRegister := true
			if rr.SignupOpensAt != nil && !rr.SignupOpensAt.IsZero() {
				opensJkt := rr.SignupOpensAt.In(locJKT)
				opensAtUnix = opensJkt.Unix()
				if nowJKT.Before(opensJkt) {
					canRegister = false
					opensIn = int64(opensJkt.Sub(nowJKT).Seconds())
				}
			}

			opts = append(opts, classOption{
				ID:             rr.ID,
				Name:           rr.Name,
				DateStr:        fmtDate(rr.Date), // your helper (Jakarta formatting)
				Capacity:       rr.Capacity,
				Confirmed:      int(rr.Confirmed),
				Waitlisted:     int(rr.Waitlisted),
				Left:           left,
				IsFull:         left == 0,
				Description:    rr.Description,
				SignupOpensAt:  rr.SignupOpensAt,
				OpensAtUnix:    opensAtUnix,
				OpensInSeconds: opensIn,
				CanRegister:    canRegister,
			})
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/parents/select_class.tmpl")
		_ = view.ExecuteTemplate(w, "parents/select_class.tmpl", map[string]any{
			"Title":        "Select Class",
			"Child":        child,
			"Parent":       parent,
			"Phone":        parent.Phone,
			"ClassOptions": opts,
			"Flash":        MakeFlash(r, errStr, ""),
		})
	}
}



func SelectClassSubmit(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		childID, _ := strconv.Atoi(r.FormValue("child_id"))
		classID, _ := strconv.Atoi(r.FormValue("class_id"))
		if childID <= 0 { http.Error(w, "missing child_id", http.StatusBadRequest); return }
		if classID <= 0 { http.Error(w, "no class selected", http.StatusBadRequest); return }

		// Load child & class
		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound); return
		}
		var class models.Class
		if err := db.Conn().First(&class, classID).Error; err != nil {
			http.Error(w, "class not found", http.StatusNotFound); return
		}

		// HARD GATE: enforce signup open time (server-side)
		if class.SignupOpensAt != nil && time.Now().Before(class.SignupOpensAt.UTC()) {
			http.Redirect(w, r,
				"/register/classes?child_id="+strconv.Itoa(childID)+"&error=not_open_yet",
				http.StatusSeeOther)
			return
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
				http.Error(w, "validation error", http.StatusBadRequest); return
			}
		}

		// >>> Robust question check: actually load questions (don’t rely on COUNT)
		var qs []models.ClassQuestion
		if err := db.Conn().
			Where("class_id = ?", class.ID).
			Order("position asc, id asc").
			Find(&qs).Error; err == nil && len(qs) > 0 {
			// send to confirm page
			http.Redirect(w, r,
				"/register/classes/confirm?child_id="+strconv.Itoa(childID)+"&class_id="+strconv.Itoa(classID),
				http.StatusSeeOther)
			return
		}

		// No questions → create the registration now (original flow)
		var confirmedCnt int64
		_ = db.Conn().Model(&models.Registration{}).
			Where("class_id = ? AND status = ?", class.ID, "confirmed").
			Count(&confirmedCnt).Error

		status := "waitlisted"
		if int(confirmedCnt) < class.Capacity { status = "confirmed" }

		code := generateRegCode()
		if code == "" {
			http.Error(w, "failed to generate code", http.StatusInternalServerError); return
		}

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

		_ = svc.RecomputeClass(uint(class.ID))
		_ = db.Conn().First(&reg, reg.ID).Error
		status = reg.Status

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
			"Title":     "Registration Result",
			"ChildName": child.Name,
			"ClassName": class.Name,
			"Date":      fmtDate(class.Date),
			"Status":    status,
			"Code":      code,
			"Rank":      rank,
		})
	}
}



// generateRegCode creates a unique REG-xxxxxx code.
func generateRegCode() string {
	for i := 0; i < 20; i++ {
		code := fmt.Sprintf("REG-%06d", rand.Intn(1000000))
		var exists int64
		_ = db.Conn().Model(&models.Registration{}).Where("code = ?", code).Count(&exists).Error
		if exists == 0 {
			return code
		}
	}
	return ""
}

func normGender(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "male", "m":
		return "male"
	case "female", "f":
		return "female"
	case "other", "o", "x":
		return "other"
	default:
		return "" // unknown / not provided
	}
}
