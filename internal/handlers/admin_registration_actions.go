package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

func redirectBack(w http.ResponseWriter, r *http.Request, fallback string) {
	ref := r.Header.Get("Referer")
	if ref == "" { ref = fallback }
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

// POST /admin/registrations/{id}/checkin
func AdminRegCheckin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404); return
	}
	if reg.Status != "confirmed" {
		http.Error(w, "only confirmed can be checked in", 400); return
	}
	now := time.Now()
	reg.CheckInAt = &now
	if err := db.Conn().Save(&reg).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	redirectBack(w, r, "/admin/roster")
}

// POST /admin/registrations/{id}/cancel
func AdminRegCancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404); return
	}
	if err := svc.CancelByCode(reg.Code); err != nil {
		http.Error(w, "unable to cancel", 500); return
	}
	redirectBack(w, r, "/admin/roster")
}

// POST /admin/registrations/{id}/delete
func AdminRegDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404); return
	}
	classID := reg.ClassID
	if err := db.Conn().Delete(&reg).Error; err != nil {
		http.Error(w, "db error", 500); return
	}
	_ = svc.RecomputeClass(classID)
	redirectBack(w, r, "/admin/roster")
}
