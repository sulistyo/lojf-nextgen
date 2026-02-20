package handlers

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type rosterRow struct {
	ID         uint
	Code       string
	Status     string
	CheckInAt  *time.Time
	CheckInStr string

	ParentID    uint
	ParentName  string
	ParentPhone string

	ChildName string
	BirthDate time.Time
	Gender    string

	ClassID   uint
	ClassName string
	ClassDate time.Time
	DateStr   string
	CreatedStr	string
	CreatedAt    time.Time
	WaitlistRank int
}

type rosterPageVM struct {
	Title     string
	Rows      []rosterRow
	Classes   []models.Class
	Filters   rosterFilters
	HasResult bool
	Answers   map[uint][]string // regID -> ["Label: Answer", ...]
}

type rosterFilters struct {
	From    string
	To      string
	ClassID string
	Status  string
	Q       string
}

func onlyDigits(s string) string {
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b = append(b, r)
		}
	}
	return string(b)
}

func parseDate(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return def
	}
	return t
}

func statusWeight(s string) int {
	switch s {
	case "confirmed":
		return 0
	case "waitlisted":
		return 1
	case "canceled":
		return 2
	default:
		return 3
	}
}

// ---------- Admin Roster (HTML) ----------
func AdminRoster(t *template.Template) http.HandlerFunc {
    view := template.Must(t.Clone())
    template.Must(view.ParseFiles("templates/pages/admin/roster.tmpl"))

    return func(w http.ResponseWriter, r *http.Request) {
        fFrom    := r.URL.Query().Get("from")
        fTo      := r.URL.Query().Get("to")
        fClassID := r.URL.Query().Get("class_id")
        fStatus  := r.URL.Query().Get("status")
        fQ       := strings.TrimSpace(r.URL.Query().Get("q"))

        // Default window: last 30 days to +300 days
        now := time.Now()
        from := parseDate(fFrom, now.AddDate(0, 0, -7))
        to   := parseDate(fTo,   now.AddDate(0, 0, 300))

		if fFrom == "" {
			fFrom = from.Format("2006-01-02")
		}
        var classes []models.Class
        _ = db.Conn().Order("date asc").Find(&classes).Error

        q := db.Conn().Table("registrations").
            Select(`registrations.id, registrations.code, registrations.status, registrations.check_in_at, registrations.created_at,
                    registrations.parent_id as parent_id,
                    children.name as child_name, children.birth_date as birth_date, children.gender as gender,
                    classes.id as class_id, classes.name as class_name, classes.date as class_date,
                    parents.name as parent_name, parents.phone as parent_phone`).
            Joins("JOIN children ON children.id = registrations.child_id").
            Joins("JOIN classes  ON classes.id  = registrations.class_id").
            Joins("JOIN parents  ON parents.id  = registrations.parent_id").
            Where("classes.date BETWEEN ? AND ?", from, to)

        if fClassID != "" {
            if cid, err := strconv.Atoi(fClassID); err == nil && cid > 0 {
                q = q.Where("classes.id = ?", cid)
            }
        }

        if fStatus != "" {
            switch fStatus {
            case "confirmed":
                // Only confirmed and NOT yet checked in
                q = q.Where("registrations.status = ? AND registrations.check_in_at IS NULL", "confirmed")
            case "waitlisted", "canceled":
                q = q.Where("registrations.status = ?", fStatus)
            case "checked-in":
                // Only confirmed and already checked in
                q = q.Where("registrations.status = ? AND registrations.check_in_at IS NOT NULL", "confirmed")
            }
        }

        if fQ != "" {
            like := "%" + strings.ToLower(fQ) + "%"
            digits := onlyDigits(fQ)
            digitsLike := "#no_digits#"
            if digits != "" {
                digitsLike = "%" + digits + "%"
            }
            where := `
                LOWER(children.name)       LIKE ? OR
                LOWER(parents.name)        LIKE ? OR
                LOWER(classes.name)        LIKE ? OR
                LOWER(registrations.code)  LIKE ? OR
                REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(parents.phone,'+',''),' ',''),'-',''),'(',''),')','') LIKE ?
            `
            q = q.Where(where, like, like, like, like, digitsLike)
        }

        var rows []rosterRow
        if err := q.Scan(&rows).Error; err != nil {
            http.Error(w, "db error", http.StatusInternalServerError)
            return
        }

        for i := range rows {
            rows[i].DateStr = fmtDate(rows[i].ClassDate)
            if rows[i].CheckInAt != nil {
                rows[i].CheckInStr = rows[i].CheckInAt.Format("15:04")
            }
        }

        sort.Slice(rows, func(i, j int) bool {
            if !rows[i].ClassDate.Equal(rows[j].ClassDate) {
                return rows[i].ClassDate.After(rows[j].ClassDate)
            }
            if rows[i].ClassName != rows[j].ClassName {
                return rows[i].ClassName < rows[j].ClassName
            }
            if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
                return rows[i].CreatedAt.After(rows[j].CreatedAt)
            }
            return statusWeight(rows[i].Status) < statusWeight(rows[j].Status)
        })
/*
        ranks := map[uint]int{}
        for i := range rows {
            if rows[i].Status == "waitlisted" {
                ranks[rows[i].ClassID]++
                rows[i].WaitlistRank = ranks[rows[i].ClassID]
            }
        }
*/

        // --- FIX WAITLIST RANK (FIFO) ---
        // Current UI order is newest-first; rank must be oldest-first.
        // Build a regID -> rank map using FIFO ordering inside each class.
        wlRankByRegID := map[uint]int{}

        if len(rows) > 0 {
            // Collect class IDs present in the result
            classIDs := make([]uint, 0, len(rows))
            seen := map[uint]bool{}
            for _, rr := range rows {
                if !seen[rr.ClassID] {
                    seen[rr.ClassID] = true
                    classIDs = append(classIDs, rr.ClassID)
                }
            }

            // Load ALL waitlisted regs for those classes in FIFO order
            // NOTE: do NOT filter by date window here; classIDs already reflect it.
            type wlRow struct {
                ID      uint
                ClassID uint
            }
            var wls []wlRow
            _ = db.Conn().Table("registrations").
                Select("id, class_id").
                Where("class_id IN ? AND status = ?", classIDs, "waitlisted").
                Order("class_id ASC, created_at ASC, id ASC").
                Scan(&wls).Error

            // Assign ranks per class in FIFO order
            perClass := map[uint]int{}
            for _, wr := range wls {
                perClass[wr.ClassID]++
                wlRankByRegID[wr.ID] = perClass[wr.ClassID]
            }

            // Attach rank back to the displayed rows
            for i := range rows {
                if rows[i].Status == "waitlisted" {
                    rows[i].WaitlistRank = wlRankByRegID[rows[i].ID]
                }
            }
        }

        
        answers := map[uint][]string{}
        if len(rows) > 0 {
            regIDs := make([]uint, 0, len(rows))
            for _, rr := range rows {
                regIDs = append(regIDs, rr.ID)
            }

            type arow struct {
                RegID    uint
                Label    string
                Answer   string
                Position int
            }
            var ans []arow
            if err := db.Conn().Table("registration_answers AS ra").
                Select("ra.registration_id AS reg_id, cq.label, ra.answer, cq.position").
                Joins("JOIN class_questions AS cq ON cq.id = ra.question_id").
                Where("ra.registration_id IN ?", regIDs).
                Order("ra.registration_id ASC, cq.position ASC, cq.id ASC").
                Scan(&ans).Error; err == nil {
                for _, a := range ans {
                    if strings.TrimSpace(a.Answer) == "" {
                        continue
                    }
                    answers[a.RegID] = append(answers[a.RegID], a.Label+": "+a.Answer)
                }
            }
        }

        vm := rosterPageVM{
            Title:   "Admin • Roster",
            Rows:    rows,
            Classes: classes,
            Filters: rosterFilters{
                From:    fFrom,
                To:      fTo,
                ClassID: fClassID,
                Status:  fStatus,
                Q:       fQ,
            },
            HasResult: len(rows) > 0,
            Answers:   answers,
        }

        if err := view.ExecuteTemplate(w, "admin/roster.tmpl", vm); err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
    }
}
func AdminRosterCSV(w http.ResponseWriter, r *http.Request) {
	fFrom    := r.URL.Query().Get("from")
	fTo      := r.URL.Query().Get("to")
	fClassID := r.URL.Query().Get("class_id")
	fStatus  := r.URL.Query().Get("status")
	fQ       := strings.TrimSpace(r.URL.Query().Get("q"))

	now  := time.Now()
	from := parseDate(fFrom, now.AddDate(0, 0, -7))
	to   := parseDate(fTo,   now.AddDate(0, 0,  300)) // MATCH AdminRoster

	type csvRow struct {
		ID          uint
		Code        string
		Status      string
		CheckInAt   *time.Time
		CreatedAt   time.Time

		ParentID    uint
		ParentName  string
		ParentPhone string

		ChildName   string
		Gender      string
		BirthDate   time.Time

		ClassID     uint
		ClassName   string
		ClassDate   time.Time
	}

	q := db.Conn().Table("registrations").
		Select(`registrations.id, registrations.code, registrations.status, registrations.check_in_at, registrations.created_at,
		        registrations.parent_id as parent_id,
		        children.name as child_name, children.gender as gender, children.birth_date as birth_date,
		        classes.id as class_id, classes.name as class_name, classes.date as class_date,
		        parents.name as parent_name, parents.phone as parent_phone`).
		Joins("JOIN children ON children.id = registrations.child_id").
		Joins("JOIN classes  ON classes.id  = registrations.class_id").
		Joins("JOIN parents  ON parents.id  = registrations.parent_id").
		Where("classes.date BETWEEN ? AND ?", from, to)

	if fClassID != "" {
		if cid, err := strconv.Atoi(fClassID); err == nil && cid > 0 {
			q = q.Where("classes.id = ?", cid)
		}
	}

	if fStatus != "" {
		switch fStatus {
		case "confirmed":
			q = q.Where("registrations.status = 'confirmed' AND registrations.check_in_at IS NULL")
		case "checked-in":
			q = q.Where("registrations.status = 'confirmed' AND registrations.check_in_at IS NOT NULL")
		case "waitlisted", "canceled":
			q = q.Where("registrations.status = ?", fStatus)
		}
	}

	if fQ != "" {
		like := "%" + strings.ToLower(fQ) + "%"
		digits := onlyDigits(fQ)
		digitsLike := "#no_digits#"
		if digits != "" {
			digitsLike = "%" + digits + "%"
		}

		where := `
			LOWER(children.name) LIKE ? OR
			LOWER(parents.name) LIKE ? OR
			LOWER(classes.name) LIKE ? OR
			LOWER(registrations.code) LIKE ? OR
			LOWER(registrations.status) LIKE ? OR
			REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(parents.phone,'+',''),' ',''),'-',''),'(',''),')','') LIKE ?
		`
		q = q.Where(where, like, like, like, like, like, digitsLike)
	}

	var rows []csvRow
	if err := q.
		Order("classes.date DESC, registrations.created_at DESC, registrations.id DESC").
		Scan(&rows).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// answers (same as your current CSV, unchanged)
	answers := map[uint][]string{}
	if len(rows) > 0 {
		regIDs := make([]uint, 0, len(rows))
		for _, rr := range rows {
			regIDs = append(regIDs, rr.ID)
		}
		type arow struct {
			RegID    uint
			Label    string
			Answer   string
			Position int
		}
		var ans []arow
		if err := db.Conn().Table("registration_answers AS ra").
			Select("ra.registration_id AS reg_id, cq.label, ra.answer, cq.position").
			Joins("JOIN class_questions AS cq ON cq.id = ra.question_id").
			Where("ra.registration_id IN ?", regIDs).
			Order("ra.registration_id ASC, cq.position ASC, cq.id ASC").
			Scan(&ans).Error; err == nil {
			for _, a := range ans {
				if strings.TrimSpace(a.Answer) == "" {
					continue
				}
				answers[a.RegID] = append(answers[a.RegID], a.Label+": "+a.Answer)
			}
		}
	}

	filename := fmt.Sprintf("roster-%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{
		"Registration Date", "Date", "Class", "Child", "Gender", "DOB",
		"Parent", "Phone", "Code", "Status", "CheckedInAt", "Answers",
	})

	for _, row := range rows {
		dateStr := row.ClassDate.Format("2006-01-02")
		createdStr := row.CreatedAt.Format("2006-01-02 15:04")
		checkStr := ""
		if row.CheckInAt != nil {
			checkStr = row.CheckInAt.Format("2006-01-02 15:04")
		}
		dobStr := ""
		if !row.BirthDate.IsZero() {
			dobStr = row.BirthDate.Format("2006-01-02")
		}
		ansStr := strings.Join(answers[row.ID], " | ")

		_ = cw.Write([]string{
			createdStr,
			dateStr,
			row.ClassName,
			row.ChildName,
			row.Gender,
			dobStr,
			row.ParentName,
			row.ParentPhone,
			row.Code,
			row.Status,
			checkStr,
			ansStr,
		})
	}
}
