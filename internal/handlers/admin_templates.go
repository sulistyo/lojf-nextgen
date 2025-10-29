package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"gorm.io/gorm"
	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// LIST
func AdminTemplatesIndex(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ts []models.ClassTemplate
		_ = db.Conn().Preload("Questions", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("position asc, id asc")
		}).Order("lower(name) asc").Find(&ts).Error

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/templates.tmpl")
		_ = view.ExecuteTemplate(w, "admin/templates.tmpl", map[string]any{
			"Title":     "Admin • Templates",
			"Templates": ts,
			"Flash":     MakeFlash(r, "", ""),
		})
	}
}

// NEW
func AdminTemplatesNewForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/templates_new.tmpl")
		_ = view.ExecuteTemplate(w, "admin/templates_new.tmpl", map[string]any{
			"Title": "Admin • New Template",
		})
	}
}

// CREATE
func AdminTemplatesCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	desc := r.FormValue("description")

	if name == "" {
		http.Redirect(w, r, "/admin/templates?error=missing", http.StatusSeeOther)
		return
	}

	tpl := models.ClassTemplate{Name: name, Description: desc}
	if err := db.Conn().Create(&tpl).Error; err != nil {
		http.Error(w, "db error", 500); return
	}

	labels := r.Form["q_label[]"]
	kinds  := r.Form["q_kind[]"]
	reqs   := r.Form["q_required[]"]
	opts   := r.Form["q_options[]"]

	reqIdx := map[int]bool{}
	for _, v := range reqs {
		if i, err := strconv.Atoi(v); err == nil { reqIdx[i] = true }
	}

	for i := 0; i < len(labels); i++ {
		lbl := strings.TrimSpace(labels[i])
		if lbl == "" { continue }
		kind := "text"
		if i < len(kinds) && (kinds[i] == "text" || kinds[i] == "radio") {
			kind = kinds[i]
		}
		opt := ""
		if kind == "radio" && i < len(opts) {
			opt = strings.TrimSpace(opts[i])
		}
		q := models.ClassTemplateQuestion{
			TemplateID: tpl.ID,
			Label:      lbl,
			Kind:       kind,
			Options:    opt,
			Required:   reqIdx[i],
			Position:   i,
		}
		_ = db.Conn().Create(&q).Error
	}

	http.Redirect(w, r, "/admin/templates?ok=saved", http.StatusSeeOther)
}

// EDIT
func AdminTemplatesEditForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))

		var tpl models.ClassTemplate
		if err := db.Conn().
			Preload("Questions", func(tx *gorm.DB) *gorm.DB {
				return tx.Order("position asc, id asc")
			}).First(&tpl, id).Error; err != nil {
			http.NotFound(w, r)
			return
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/templates_edit.tmpl")
		_ = view.ExecuteTemplate(w, "admin/templates_edit.tmpl", map[string]any{
			"Title":     "Admin • Edit Template",
			"Tpl":       tpl,
			"Questions": tpl.Questions, // convenience alias
		})
	}
}


// UPDATE
func AdminTemplatesUpdate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idStr := chi.URLParam(r, "id")
	tid, _ := strconv.Atoi(idStr)

	var tpl models.ClassTemplate
	if err := db.Conn().First(&tpl, tid).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// Update template fields
	tpl.Name = strings.TrimSpace(r.FormValue("name"))
	tpl.Description = strings.TrimSpace(r.FormValue("description"))
	if err := db.Conn().Save(&tpl).Error; err != nil {
		http.Error(w, "db error (template)", http.StatusInternalServerError)
		return
	}

	// Parallel arrays for questions
	qIDs     := r.Form["q_id[]"]
	qLabels  := r.Form["q_label[]"]
	qKinds   := r.Form["q_kind[]"]     // "text" | "radio"
	qChoices := r.Form["q_choices[]"]  // comma-separated for radio
	qPos     := r.Form["q_position[]"]
	qDel     := r.Form["q_delete[]"]   // "1" if delete

	for i := range qLabels {
		idStr   := at(qIDs, i)
		label   := strings.TrimSpace(at(qLabels, i))
		kind    := strings.TrimSpace(at(qKinds, i))
		choices := normalizeChoicesComma(at(qChoices, i))
		posStr  := strings.TrimSpace(at(qPos, i))
		del     := at(qDel, i) == "1"
		req     := r.FormValue("q_required_"+strconv.Itoa(i)) == "on"

		position := 0
		if posStr != "" {
			if n, err := strconv.Atoi(posStr); err == nil {
				position = n
			}
		}
		// ignore totally empty rows
		if idStr == "" && !del && label == "" && kind == "" && choices == "" {
			continue
		}

		if idStr != "" {
			// update / delete existing
			qid, _ := strconv.Atoi(idStr)
			var q models.ClassQuestion
			if err := db.Conn().First(&q, qid).Error; err == nil && q.ID > 0 {
				if del {
					_ = db.Conn().Delete(&q).Error
					continue
				}
				if kind != "radio" {
					choices = ""
				}
				q.Label     = label
				q.Kind      = kind
				q.Options   = choices
				q.Required  = req
				q.Position  = position
				q.ClassID   = nil
				q.TemplateID = &tpl.ID
				_ = db.Conn().Save(&q).Error
			}
		} else {
			// new question
			if del || label == "" {
				continue
			}
			if kind != "radio" {
				choices = ""
			}
			q := models.ClassQuestion{
				Label:      label,
				Kind:       kind,
				Options:    choices,
				Required:   req,
				Position:   position,
				ClassID:    nil,
				TemplateID: &tpl.ID,
			}
			_ = db.Conn().Create(&q).Error
		}
	}

	http.Redirect(w, r, "/admin/templates/"+idStr+"/edit?ok=saved", http.StatusSeeOther)
}

// DELETE
func AdminTemplatesDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	// cascade delete questions first
	_ = db.Conn().Where("template_id = ?", id).Delete(&models.ClassTemplateQuestion{}).Error
	_ = db.Conn().Delete(&models.ClassTemplate{}, id).Error
	http.Redirect(w, r, "/admin/templates?ok=deleted", http.StatusSeeOther)
}

// JSON (for prefill in Create Class form)
func AdminTemplatesShowJSON(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var tpl models.ClassTemplate
	if err := db.Conn().First(&tpl, id).Error; err != nil {
		http.NotFound(w, r); return
	}
	var qs []models.ClassTemplateQuestion
	_ = db.Conn().Where("template_id = ?", tpl.ID).Order("position asc, id asc").Find(&qs).Error

	type jq struct {
		ID          uint   `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Questions   []struct {
			Label    string `json:"label"`
			Kind     string `json:"kind"`
			Options  string `json:"options"`
			Required bool   `json:"required"`
			Position int    `json:"position"`
		} `json:"questions"`
	}
	out := jq{ID: tpl.ID, Name: tpl.Name, Description: tpl.Description}
	for _, q := range qs {
		out.Questions = append(out.Questions, struct {
			Label    string `json:"label"`
			Kind     string `json:"kind"`
			Options  string `json:"options"`
			Required bool   `json:"required"`
			Position int    `json:"position"`
		}{
			Label: q.Label, Kind: q.Kind, Options: q.Options, Required: q.Required, Position: q.Position,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}


func normalizeChoicesComma(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}
