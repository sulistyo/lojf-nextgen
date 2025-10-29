package handlers

import (
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
	"gorm.io/gorm"
)

func AdminClasses(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var classes []models.Class
		if err := db.Conn().Order("date asc").Find(&classes).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}
		view, err := t.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := view.ParseFiles("templates/pages/admin/classes.tmpl"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		data := map[string]any{"Title": "Admin • Classes", "Classes": classes}
		if err := view.ExecuteTemplate(w, "admin/classes.tmpl", data); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
}


func AdminNewClass(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load templates (with questions ordered by position)
		var tpls []models.ClassTemplate
		_ = db.Conn().
			Preload("Questions", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("position asc, id asc")
			}).
			Order("LOWER(name) asc").
			Find(&tpls).Error

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/classes_new.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		data := map[string]any{
			"Title": "Admin • New Class",
			"Tpls":  tpls, // <-- pass templates to the page
		}
		if err := view.ExecuteTemplate(w, "admin/classes_new.tmpl", data); err != nil {
			http.Error(w, err.Error(), 500); return
		}
	}
}


func AdminCreateClass(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest); return
	}

	dateStr := r.FormValue("date")
	name    := r.FormValue("name")
	capStr  := r.FormValue("capacity")
	desc    := r.FormValue("description")

	openDate := r.FormValue("open_date")
	openTime := r.FormValue("open_time")

	if dateStr == "" || name == "" || capStr == "" {
		http.Error(w, "missing fields", http.StatusBadRequest); return
	}
	d, err := time.Parse("2006-01-02", dateStr)
	if err != nil { http.Error(w, "invalid date", http.StatusBadRequest); return }

	capacity, err := strconv.Atoi(capStr)
	if err != nil || capacity < 0 {
		http.Error(w, "invalid capacity", http.StatusBadRequest); return
	}

	opensAt, err := parseOptionalJakartaDateTime(openDate, openTime)
	if err != nil {
		http.Error(w, "invalid opens-at", http.StatusBadRequest); return
	}

	cl := models.Class{
		Date:          d,
		Name:          name,
		Capacity:      capacity,
		Description:   strings.TrimSpace(desc),
		SignupOpensAt: opensAt,
	}
	if err := db.Conn().Create(&cl).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError); return
	}

	// ------- Custom Questions -------
	labels := r.Form["q_label[]"]
	kinds  := r.Form["q_kind[]"]        // "text" | "radio"
	reqs   := r.Form["q_required[]"]    // checkbox indices as strings

	// Support BOTH names for choices to match your forms
	optsA  := r.Form["q_options[]"]     // from classes_new.tmpl
	optsB  := r.Form["q_choices[]"]     // from edit page

	// Map of required indices
	reqIdx := map[int]bool{}
	for _, v := range reqs {
		if i, err := strconv.Atoi(v); err == nil { reqIdx[i] = true }
	}

	n := len(labels)
	for i := 0; i < n; i++ {
		lbl := strings.TrimSpace(labels[i])
		if lbl == "" { continue }

		kind := "text"
		if i < len(kinds) && (kinds[i] == "text" || kinds[i] == "radio") {
			kind = kinds[i]
		}

		// pull raw choices from whichever array exists
		raw := ""
		if i < len(optsA) { raw = optsA[i] } else if i < len(optsB) { raw = optsB[i] }
		choices := normalizeChoices(raw) // "Yes, No, Maybe"

		if kind != "radio" { choices = "" } // only store for radio

		q := models.ClassQuestion{
			ClassID:  &cl.ID,
			Label:    lbl,
			Kind:     kind,
			Options:  choices,     // comma-separated
			Required: reqIdx[i],
			Position: i,
		}
		_ = db.Conn().Create(&q).Error
	}

	http.Redirect(w, r, "/admin/classes?ok=saved", http.StatusSeeOther)
}



// parseOptionalJakartaDateTime("2006-01-02","15:04") -> *time.Time or nil
func parseOptionalJakartaDateTime(dateStr, timeStr string) (*time.Time, error) {
	if strings.TrimSpace(dateStr) == "" {
		return nil, nil
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	layout := "2006-01-02"
	if strings.TrimSpace(timeStr) != "" {
		layout = "2006-01-02 15:04"
		dateStr = dateStr + " " + timeStr
	}
	t, err := time.ParseInLocation(layout, dateStr, loc)
	if err != nil {
		return nil, err
	}
	ut := t.UTC()
	return &ut, nil
}
