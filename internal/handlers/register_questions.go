package handlers

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

// View model for questions
type qVM struct {
	ID       uint
	Label    string
	Kind     string
	Required bool
	Choices  []string
}

func SelectClassConfirmForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/class_confirm.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		childID, _ := strconv.Atoi(r.URL.Query().Get("child_id"))
		classID, _ := strconv.Atoi(r.URL.Query().Get("class_id"))
		if childID == 0 || classID == 0 {
			http.Error(w, "missing ids", http.StatusBadRequest)
			return
		}

		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound)
			return
		}
		var class models.Class
		if err := db.Conn().First(&class, classID).Error; err != nil {
			http.Error(w, "class not found", http.StatusNotFound)
			return
		}

		var qs []models.ClassQuestion
		_ = db.Conn().Where("class_id = ?", classID).Order("position asc, id asc").Find(&qs).Error

		items := make([]qVM, 0, len(qs))
		for _, q := range qs {
			v := qVM{ID: q.ID, Label: q.Label, Kind: q.Kind, Required: q.Required}
			if q.Kind == "radio" && strings.TrimSpace(q.Options) != "" {
				parts := strings.Split(q.Options, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				v.Choices = parts
			}
			items = append(items, v)
		}

		if err := view.ExecuteTemplate(w, "parents/class_confirm.tmpl", map[string]any{
			"Title":   "Confirm Registration",
			"Child":   child,
			"Class":   class,
			"Qs":      items,
			"ChildID": childID,
			"ClassID": classID,
			"Err":     r.URL.Query().Get("err"),
		}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

	}
}

func SelectClassConfirmSubmit(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/parents/registration_done.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		childID, _ := strconv.Atoi(r.FormValue("child_id"))
		classID, _ := strconv.Atoi(r.FormValue("class_id"))
		if childID == 0 || classID == 0 {
			http.Error(w, "missing ids", http.StatusBadRequest)
			return
		}

		// Load questions to validate
		var qs []models.ClassQuestion
		_ = db.Conn().Where("class_id = ?", classID).Order("position asc, id asc").Find(&qs).Error

		answers := make(map[uint]string, len(qs))
		for _, q := range qs {
			key := "q_" + strconv.FormatUint(uint64(q.ID), 10)
			val := strings.TrimSpace(r.FormValue(key))
			if q.Required && val == "" {
				http.Redirect(w, r,
					"/register/classes/confirm?child_id="+strconv.Itoa(childID)+"&class_id="+strconv.Itoa(classID)+"&err="+url.QueryEscape("Please answer: "+q.Label),
					http.StatusSeeOther)
				return
			}
			if q.Kind == "radio" && val != "" && strings.TrimSpace(q.Options) != "" {
				ok := false
				for _, opt := range strings.Split(q.Options, ",") {
					if strings.TrimSpace(opt) == val {
						ok = true
						break
					}
				}
				if !ok {
					http.Redirect(w, r,
						"/register/classes/confirm?child_id="+strconv.Itoa(childID)+"&class_id="+strconv.Itoa(classID)+"&err="+url.QueryEscape("Invalid choice for: "+q.Label),
						http.StatusSeeOther)
					return
				}
			}
			answers[q.ID] = val
		}

		// Safety: conflicts again
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

		// Create registration (same logic as your SelectClassSubmit)
		var child models.Child
		if err := db.Conn().First(&child, childID).Error; err != nil {
			http.Error(w, "child not found", http.StatusNotFound)
			return
		}
		var class models.Class
		if err := db.Conn().First(&class, classID).Error; err != nil {
			http.Error(w, "class not found", http.StatusNotFound)
			return
		}

		if class.SignupOpensAt != nil {
		    nowUTC := time.Now().UTC()
		    // allow a small grace to cover click at zero / network delay
		    opensUTC := class.SignupOpensAt.UTC().Add(-2 * time.Second)
		    if nowUTC.Before(opensUTC) {
		        http.Redirect(w, r,
		            "/register/classes?child_id="+strconv.Itoa(childID)+"&error=not_open_yet",
		            http.StatusSeeOther)
		        return
		    }
		}

		var confirmedCnt int64
		_ = db.Conn().Model(&models.Registration{}).
			Where("class_id = ? AND status = ?", class.ID, "confirmed").Count(&confirmedCnt).Error
		status := "waitlisted"
		if int(confirmedCnt) < class.Capacity {
			status = "confirmed"
		}

		code := generateRegCode()
		if code == "" {
			http.Error(w, "failed to generate code", http.StatusInternalServerError)
			return
		}

		var reg models.Registration
		err := db.Conn().Transaction(func(tx *gorm.DB) error {
			reg = models.Registration{
				ParentID: child.ParentID,
				ChildID:  child.ID,
				ClassID:  class.ID,
				Status:   status,
				Code:     code,
			}
			if err := tx.Create(&reg).Error; err != nil {
				return err
			}

			// Save answers
			for qid, ans := range answers {
				ra := models.RegistrationAnswer{
					RegistrationID: reg.ID,
					QuestionID:     qid,
					Answer:         ans,
				}
				if err := tx.Create(&ra).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		// Recompute & maybe update status
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
