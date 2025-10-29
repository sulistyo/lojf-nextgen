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
	"gorm.io/gorm"
)

type parentRow struct {
	ID           uint
	Name         string
	Phone        string
	Children     int64
	UpcomingRegs int64
}

// internal/handlers/admin_parents.go
// internal/handlers/admin_parents.go
func AdminParentsList(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		per, _ := strconv.Atoi(r.URL.Query().Get("per"))
		if page < 1 {
			page = 1
		}
		if per < 1 || per > 200 {
			per = 25
		}
		offset := (page - 1) * per

		countQ := db.Conn().Model(&models.Parent{})
		listQ := db.Conn().Model(&models.Parent{})

		if q != "" {
			like := "%" + strings.ToLower(q) + "%"

			// digits-only variant for phone
			digits := q
			for _, ch := range []string{" ", "-", "(", ")", "+"} {
				digits = strings.ReplaceAll(digits, ch, "")
			}

			// Parents whose children match the query (by name)
			var childParentIDs []uint
			_ = db.Conn().Model(&models.Child{}).
				Distinct("parent_id").
				Where("LOWER(name) LIKE ?", like).
				Pluck("parent_id", &childParentIDs)

			// Base where: parent name/phone/email
			where := `
				LOWER(name)  LIKE ? OR
				LOWER(phone) LIKE ? OR
				REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(phone,'+',''),' ',''),'-',''),'(',''),')','') LIKE ? OR
				LOWER(email) LIKE ?
			`
			args := []any{like, like, "%" + digits + "%", like}

			// Add child-name hit (parent id in subquery) if any
			if len(childParentIDs) > 0 {
				where += " OR id IN ?"
				args = append(args, childParentIDs)
			}

			countQ = countQ.Where(where, args...)
			listQ = listQ.Where(where, args...)
		}

		var total int64
		if err := countQ.Count(&total).Error; err != nil {
			http.Error(w, "db error (count)", http.StatusInternalServerError)
			return
		}

		var parents []models.Parent
		if err := listQ.
			Order("LOWER(name) asc").
			Limit(per).
			Offset(offset).
			Find(&parents).Error; err != nil {
			http.Error(w, "db error (list)", http.StatusInternalServerError)
			return
		}

		// ---- Build "Children (age)" summary per parent ----
		childAges := map[uint]string{}
		if len(parents) > 0 {
			ids := make([]uint, 0, len(parents))
			for _, p := range parents {
				ids = append(ids, p.ID)
			}

			type kidRow struct {
				ParentID  uint
				Name      string
				BirthDate time.Time
			}
			var kids []kidRow
			if err := db.Conn().Model(&models.Child{}).
				Select("parent_id, name, birth_date").
				Where("parent_id IN ?", ids).
				Order("name asc").
				Scan(&kids).Error; err == nil {

				group := make(map[uint][]kidRow, len(ids))
				for _, k := range kids {
					group[k.ParentID] = append(group[k.ParentID], k)
				}

				loc, _ := time.LoadLocation("Asia/Jakarta")
				ageYears := func(dob time.Time) string {
					if dob.IsZero() {
						return ""
					}
					now := time.Now().In(loc)
					y := now.Year() - dob.In(loc).Year()
					anniv := time.Date(now.Year(), dob.In(loc).Month(), dob.In(loc).Day(), 0, 0, 0, 0, loc)
					if now.Before(anniv) {
						y--
					}
					if y < 0 {
						y = 0
					}
					return strconv.Itoa(y)
				}

				for pid, arr := range group {
					parts := make([]string, 0, len(arr))
					for _, k := range arr {
						if k.BirthDate.IsZero() {
							parts = append(parts, k.Name)
						} else {
							parts = append(parts, k.Name+" ("+ageYears(k.BirthDate)+")")
						}
					}
					childAges[pid] = strings.Join(parts, ", ")
				}
			}
		}

		type vm struct {
			Title     string
			Q         string
			Page      int
			Per       int
			Total     int64
			Parents   []models.Parent
			HasPrev   bool
			HasNext   bool
			PrevPage  int
			NextPage  int
			ChildAges map[uint]string
			Flash     *Flash
		}
		v := vm{
			Title:     "Admin • Parents",
			Q:         q,
			Page:      page,
			Per:       per,
			Total:     total,
			Parents:   parents,
			HasPrev:   page > 1,
			HasNext:   int64(offset+per) < total,
			PrevPage:  page - 1,
			NextPage:  page + 1,
			ChildAges: childAges,
			Flash:     MakeFlash(r, "", ""),
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/parents.tmpl")
		_ = view.ExecuteTemplate(w, "admin/parents.tmpl", v)
	}
}

func AdminParentShowForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))

		var parent models.Parent
		if err := db.Conn().First(&parent, id).Error; err != nil {
			http.NotFound(w, r)
			return
		}
		var kids []models.Child
		_ = db.Conn().Where("parent_id = ?", parent.ID).Order("name asc").Find(&kids).Error

		// Legacy ?err=has_future support → convert to a human message via MakeFlash’s errStr.
		errMsg := ""
		if r.URL.Query().Get("err") == "has_future" {
			errMsg = "Cannot delete: parent has upcoming registrations. Cancel them first."
		}

		view, _ := t.Clone()
		_, _ = view.ParseFiles("templates/pages/admin/parent_show.tmpl")
		_ = view.ExecuteTemplate(w, "admin/parent_show.tmpl", map[string]any{
			"Title":  "Admin • Parent",
			"Parent": parent,
			"Kids":   kids,
			"Flash":  MakeFlash(r, errMsg, ""), // unifies ?ok=… / ?error=… / errMsg
		})
	}
}

// POST /admin/parents/{id}
func AdminParentUpdate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	idStr := chi.URLParam(r, "id")
	pid, err := strconv.Atoi(idStr)
	if err != nil || pid <= 0 {
		http.NotFound(w, r)
		return
	}

	// Load the parent
	var parent models.Parent
	if err := db.Conn().First(&parent, pid).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// Read inputs
	nameIn := strings.TrimSpace(r.FormValue("parent_name"))
	phoneIn := strings.TrimSpace(r.FormValue("phone"))
	emailRaw := r.FormValue("email")

	// Normalize/validate
	email, ok := svc.NormEmail(emailRaw) // "" allowed
	if !ok {
		http.Redirect(w, r, "/admin/parents/"+idStr+"?error=invalid_email", http.StatusSeeOther)
		return
	}
	var phone string
	if phoneIn != "" {
		phone = svc.NormPhone(phoneIn)
	}

	// Preserve existing values if fields are omitted
	if nameIn == "" {
		nameIn = parent.Name
	}
	if phone == "" {
		phone = parent.Phone
	}
	if nameIn == "" || phone == "" {
		http.Redirect(w, r, "/admin/parents/"+idStr+"?error=missing", http.StatusSeeOther)
		return
	}

	// Apply & save
	parent.Name = nameIn
	parent.Phone = phone
	parent.Email = email // string field; empty string means unset

	if err := db.Conn().Save(&parent).Error; err != nil {
		le := strings.ToLower(err.Error())
		if strings.Contains(le, "unique") && strings.Contains(le, "email") {
			http.Redirect(w, r, "/admin/parents/"+idStr+"?error=email_in_use", http.StatusSeeOther)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/parents/"+idStr+"?ok=saved", http.StatusSeeOther)
}

func AdminChildUpdate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	parentID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	childID, _ := strconv.Atoi(r.FormValue("child_id"))
	name := r.FormValue("child_name")
	dob := r.FormValue("child_dob")
	gender := normGender(r.FormValue("child_gender")) // NEW

	if childID == 0 || name == "" {
		http.Error(w, "missing child_id or name", http.StatusBadRequest); return
	}

	var child models.Child
	if err := db.Conn().First(&child, childID).Error; err != nil {
		http.NotFound(w, r); return
	}

	child.Name = name
	if dob != "" {
		if d, err := time.Parse("2006-01-02", dob); err == nil {
			child.BirthDate = d
		} else {
			http.Error(w, "invalid date", http.StatusBadRequest); return
		}
	}
	child.Gender = gender // NEW

	if err := db.Conn().Save(&child).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError); return
	}

	http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(parentID)+"?ok=child_saved", http.StatusSeeOther)
}


func AdminChildDelete(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	parentID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	childID, _ := strconv.Atoi(r.FormValue("child_id"))
	if childID == 0 {
		http.Error(w, "missing child_id", 400)
		return
	}
	var child models.Child
	if err := db.Conn().First(&child, childID).Error; err != nil {
		http.NotFound(w, r)
		return
	}
	if err := db.Conn().Delete(&child).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(parentID)+"?ok=child_deleted", http.StatusSeeOther)
}

// POST /admin/parents/{id}/delete
func AdminParentDelete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	parentID, _ := strconv.Atoi(idStr)
	if parentID <= 0 {
		http.Error(w, "invalid parent id", http.StatusBadRequest)
		return
	}

	// ---- SAFETY GUARD: block deletion if there are upcoming registrations
	var future int64
	if err := db.Conn().Table("registrations").
		Joins("JOIN classes ON classes.id = registrations.class_id").
		Where("registrations.parent_id = ? AND classes.date >= ?", parentID, time.Now()).
		Count(&future).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if future > 0 {
		// bounce back to detail page with error banner
		http.Redirect(w, r, "/admin/parents/"+strconv.Itoa(parentID)+"?err=has_future", http.StatusSeeOther)
		return
	}
	// ---------------------------------------

	// Gather impacted classes (to recompute waitlists after delete)
	var regs []models.Registration
	if err := db.Conn().Where("parent_id = ?", parentID).Find(&regs).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	classSet := map[uint]struct{}{}
	for _, r := range regs {
		classSet[r.ClassID] = struct{}{}
	}

	// Delete registrations, children, parent atomically
	if err := db.Conn().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("parent_id = ?", parentID).Delete(&models.Registration{}).Error; err != nil {
			return err
		}
		if err := tx.Where("parent_id = ?", parentID).Delete(&models.Child{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Parent{}, parentID).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Best-effort recompute for affected classes
	for cid := range classSet {
		_ = svc.RecomputeClass(cid)
	}

	http.Redirect(w, r, "/admin/parents?ok=deleted", http.StatusSeeOther)
}
