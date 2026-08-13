package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

func redirectBack(w http.ResponseWriter, r *http.Request, fallback string) {
	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = fallback
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

// POST /admin/registrations/{id}/checkin
//
// Shared by the volunteer station and the admin roster. The check-in role is
// fenced to today's classes at its own campus; admins are not.
func AdminRegCheckin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if reg.Status != "confirmed" {
		checkinFail(w, r, "only_confirmed")
		return
	}
	var class models.Class
	if err := db.Conn().First(&class, reg.ClassID).Error; err != nil {
		http.Error(w, "class not found", 404)
		return
	}
	if err := guardCheckin(CurrentUser(r), class); err != nil {
		writeAudit(r, nil, "registration.checkin.denied", regTarget(reg), err.Error())
		checkinFail(w, r, "not_allowed")
		return
	}
	if reg.CheckInAt != nil {
		checkinFail(w, r, "already_checkedin")
		return
	}

	now := time.Now()
	reg.CheckInAt = &now
	reg.CheckedInBy = actorLabel(r)
	if err := db.Conn().Save(&reg).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	writeAudit(r, nil, "registration.checkin", regTarget(reg), "class:"+class.Name)
	redirectBack(w, r, "/admin/roster")
}

func regTarget(reg models.Registration) string {
	return fmt.Sprintf("registration:%d (%s)", reg.ID, reg.Code)
}

// checkinFail sends the caller back where they came from with an error flag,
// so the station shows a red flash instead of a bare 400 page.
func checkinFail(w http.ResponseWriter, r *http.Request, reason string) {
	ref := r.Header.Get("Referer")
	if strings.Contains(ref, "/station") {
		http.Redirect(w, r, "/station?error="+reason, http.StatusSeeOther)
		return
	}
	http.Error(w, reason, http.StatusBadRequest)
}

// POST /admin/registrations/{id}/cancel
func AdminRegCancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if err := svc.CancelByCode(reg.Code); err != nil {
		http.Error(w, "unable to cancel", 500)
		return
	}
	writeAudit(r, nil, "registration.cancel", regTarget(reg), "")
	redirectBack(w, r, "/admin/roster")
}

// POST /admin/registrations/{id}/delete
func AdminRegDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg models.Registration
	if err := db.Conn().First(&reg, id).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	classID := reg.ClassID
	if err := db.Conn().Delete(&reg).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	writeAudit(r, nil, "registration.delete", regTarget(reg), "")
	_ = svc.RecomputeClass(classID)
	redirectBack(w, r, "/admin/roster")
}
