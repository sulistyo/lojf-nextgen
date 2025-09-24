package handlers

import (
	"html/template"
	"net/http"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

func CancelForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/cancel.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}

		if code == "" {
			if err := view.ExecuteTemplate(w, "parents/cancel.tmpl", map[string]any{
				"Title": "Cancel Registration", "Err": "Missing code.",
			}); err != nil { http.Error(w, err.Error(), 500) }
			return
		}

		var reg models.Registration
		if err := db.Conn().Where("code = ?", code).First(&reg).Error; err != nil {
			if err := view.ExecuteTemplate(w, "parents/cancel.tmpl", map[string]any{
				"Title": "Cancel Registration", "Err": "Code not found.",
			}); err != nil { http.Error(w, err.Error(), 500) }
			return
		}
		var child models.Child
		_ = db.Conn().First(&child, reg.ChildID).Error
		var class models.Class
		_ = db.Conn().First(&class, reg.ClassID).Error

		if err := view.ExecuteTemplate(w, "parents/cancel.tmpl", map[string]any{
			"Title":  "Cancel Registration",
			"Code":   code,
			"Child":  child.Name,
			"Class":  class.Name,
			"Date":   class.Date.Format("Mon, 02 Jan 2006 15:04"),
			"Status": reg.Status,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}

func CancelSubmit(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil { http.Error(w, err.Error(), 400); return }
		code := r.FormValue("code")
		if code == "" { http.Error(w, "missing code", 400); return }

		if err := svc.CancelByCode(code); err != nil {
			http.Error(w, "unable to cancel: "+err.Error(), 500); return
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/parents/cancel_done.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		if err := view.ExecuteTemplate(w, "parents/cancel_done.tmpl", map[string]any{
			"Title": "Canceled", "Code": code,
		}); err != nil { http.Error(w, err.Error(), 500) }
	}
}
