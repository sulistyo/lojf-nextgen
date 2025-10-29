package handlers

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

// GET /admin/classes/{id}/edit
func AdminEditClassForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))

		// Load class first
		var class models.Class
		if err := db.Conn().First(&class, id).Error; err != nil {
			http.NotFound(w, r)
			return
		}

		// Compute opens-at (Jakarta) formatted values for the form
		var openDateVal, openTimeVal string
		if class.SignupOpensAt != nil {
			jkt := class.SignupOpensAt.In(time.FixedZone("WIB", 7*3600))
			openDateVal = jkt.Format("2006-01-02")
			openTimeVal = jkt.Format("15:04")
		}

		// Load existing questions (ordered)
		var qs []models.ClassQuestion
		_ = db.Conn().
			Where("class_id = ?", class.ID).
			Order("position asc, id asc").
			Find(&qs).Error

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/classes_edit.tmpl")
		_ = view.ExecuteTemplate(w, "admin/classes_edit.tmpl", map[string]any{
			"Title":       "Admin • Edit Class",
			"Class":       class,
			"DateVal":     class.Date.Format("2006-01-02"),
			"TimeVal":     class.Date.Format("15:04"),
			"OpenDateVal": openDateVal,
			"OpenTimeVal": openTimeVal,
			"Questions":   qs,
		})
	}
}

// POST /admin/classes/{id}
func AdminUpdateClass(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))

	// Load class
	var class models.Class
	if err := db.Conn().First(&class, id).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// ----- Class fields -----
	name := r.FormValue("name")
	date := r.FormValue("date")     // YYYY-MM-DD
	timeStr := r.FormValue("time")  // optional HH:MM
	capStr := r.FormValue("capacity")
	desc := r.FormValue("description")

	// Optional opens-at (Jakarta)
	openDate := r.FormValue("open_date")
	openTime := r.FormValue("open_time")

	// Compose datetime for class date/time (Jakarta or UTC? We keep as parsed local then store as time with no TZ consideration like before)
	dtStr := date
	if strings.TrimSpace(timeStr) != "" {
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

	// Parse optional opens-at in Asia/Jakarta; store as UTC
	var opensAt *time.Time
	if strings.TrimSpace(openDate) != "" {
		loc, _ := time.LoadLocation("Asia/Jakarta")
		layout := "2006-01-02"
		combined := openDate
		if strings.TrimSpace(openTime) != "" {
			layout = "2006-01-02 15:04"
			combined = openDate + " " + openTime
		}
		if t, err := time.ParseInLocation(layout, combined, loc); err == nil {
			ut := t.UTC()
			opensAt = &ut
		} else {
			http.Error(w, "invalid opens-at", http.StatusBadRequest)
			return
		}
	}

	// Save class core fields
	class.Name = name
	class.Date = dt
	class.Capacity = capacity
	class.Description = desc
	class.SignupOpensAt = opensAt

	if err := db.Conn().Save(&class).Error; err != nil {
		http.Error(w, "db error (class)", http.StatusInternalServerError)
		return
	}

	// ----- Questions: create/update/delete -----
	// Expect: q_id[], q_label[], q_kind[], q_choices[], q_position[], q_delete[]
	// And checkboxes: q_required_{index} = "on" when checked.

	qIDs := r.Form["q_id[]"]
	qLabels := r.Form["q_label[]"]
	qKinds := r.Form["q_kind[]"]
	qChoices := r.Form["q_choices[]"]
	qPos := r.Form["q_position[]"]
	qDel := r.Form["q_delete[]"]

	for i := range qLabels {
		idStr := at(qIDs, i)
		label := strings.TrimSpace(at(qLabels, i))
		kind := strings.TrimSpace(at(qKinds, i)) // "text" | "radio"
		opts := normalizeChoices(at(qChoices, i)) // "Yes, No" (trimmed, comma-separated)
		posStr := strings.TrimSpace(at(qPos, i))
		del := at(qDel, i) == "1"
		req := r.FormValue("q_required_" + strconv.Itoa(i)) == "on"

		position := 0
		if posStr != "" {
			if n, err := strconv.Atoi(posStr); err == nil {
				position = n
			}
		}

		// skip empty noop rows
		if idStr == "" && !del && label == "" && kind == "" && opts == "" {
			continue
		}

		// sanitize kind
		if kind != "radio" && kind != "text" {
			kind = "text"
		}
		// if not radio, ignore options
		if kind != "radio" {
			opts = ""
		}

		if idStr != "" {
			// Update/Delete existing
			qid, _ := strconv.Atoi(idStr)
			var q models.ClassQuestion
			if err := db.Conn().First(&q, qid).Error; err == nil && q.ID > 0 {
				if del {
					_ = db.Conn().Delete(&q).Error
					continue
				}
				q.Label = label
				q.Kind = kind
				q.Options = opts
				q.Required = req
				q.Position = position
				_ = db.Conn().Save(&q).Error
			}
		} else {
			// New row (ignore if deleting or missing label)
			if del || label == "" {
				continue
			}
			q := models.ClassQuestion{
				ClassID:  &class.ID,
				Label:    label,
				Kind:     kind,
				Options:  opts,
				Required: req,
				Position: position,
			}
			_ = db.Conn().Create(&q).Error
		}
	}

	// Re-balance registrations for this class
	_ = svc.RecomputeClass(uint(class.ID))

	http.Redirect(w, r, "/admin/classes?ok=saved", http.StatusSeeOther)
}

// --------- helpers (single definitions) ---------

// safe index access on parallel arrays
func at(arr []string, i int) string {
	if i >= 0 && i < len(arr) {
		return arr[i]
	}
	return ""
}

// normalizeChoices turns "Yes ,No,  Maybe" or multi-line into "Yes, No, Maybe"
func normalizeChoices(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	// support both comma-separated and newline-separated authoring
	s = strings.ReplaceAll(s, "\n", ",")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}
