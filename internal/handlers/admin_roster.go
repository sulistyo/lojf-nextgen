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
	"log"                 // <-- add
	"gorm.io/gorm" 
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
	ChildName   string
	BirthDate  time.Time 
	Gender     string    
	ClassID     uint
	ClassName   string
	ClassDate   time.Time
	DateStr     string

	CreatedAt    time.Time
	WaitlistRank int
}

type rosterPageVM struct {
	Title     string
	Rows      []rosterRow
	Classes   []models.Class
	Filters   rosterFilters
	HasResult bool

	Answers map[uint][]string // regID -> ["Label: Answer", ...]
}

type rosterFilters struct {
	From    string
	To      string
	ClassID string
	Status  string
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
	return func(w http.ResponseWriter, r *http.Request) {
		fFrom    := r.URL.Query().Get("from")
		fTo      := r.URL.Query().Get("to")
		fClassID := r.URL.Query().Get("class_id")
		fStatus  := r.URL.Query().Get("status")

		now  := time.Now()
		from := parseDate(fFrom, now.AddDate(0, 0, -30))
		to   := parseDate(fTo,   now.AddDate(0, 0,  300))

		var classes []models.Class
		_ = db.Conn().Order("date asc").Find(&classes).Error

		q := db.Conn().Table("registrations").
			Select(`registrations.id, registrations.code, registrations.status, registrations.check_in_at, registrations.created_at,
			        registrations.parent_id as parent_id,
			        children.name as child_name,children.birth_date as birth_date,children.gender as gender,
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
			case "confirmed", "waitlisted", "canceled":
				q = q.Where("registrations.status = ?", fStatus)
			case "checked-in":
				q = q.Where("registrations.check_in_at IS NOT NULL")
			}
		}


if r.URL.Query().Get("debug") == "1" {
    dry := db.Conn().Session(&gorm.Session{DryRun: true}).Table("registrations").
        Select(`registrations.id, registrations.code, registrations.status, registrations.check_in_at, registrations.created_at,
                registrations.parent_id as parent_id,
                children.name as child_name,
                classes.id as class_id, classes.name as class_name, classes.date as class_date,
                parents.name as parent_name, parents.phone as parent_phone`).
        Joins("JOIN children ON children.id = registrations.child_id").
        Joins("JOIN classes  ON classes.id  = registrations.class_id").
        Joins("JOIN parents  ON parents.id  = registrations.parent_id").
        Where("classes.date BETWEEN ? AND ?", from, to)

    // Re-apply the same filters you added to q
    if fClassID != "" {
        if cid, err := strconv.Atoi(fClassID); err == nil && cid > 0 {
            dry = dry.Where("classes.id = ?", cid)
        }
    }
    if fStatus != "" {
        switch fStatus {
        case "confirmed", "waitlisted", "canceled":
            dry = dry.Where("registrations.status = ?", fStatus)
        case "checked-in":
            dry = dry.Where("registrations.check_in_at IS NOT NULL")
        }
    }

    // Trigger statement build
    tx := dry.Scan(&[]rosterRow{})
    log.Printf("ROSTER SQL:\n%s\nARGS:\n%#v", tx.Statement.SQL.String(), tx.Statement.Vars)
}

		var rows []rosterRow
		if err := q.Scan(&rows).Error; err != nil {
			http.Error(w, "db error", http.StatusInternalServerError); return
		}

		for i := range rows {
			rows[i].DateStr = fmtDate(rows[i].ClassDate)
			if rows[i].CheckInAt != nil {
				rows[i].CheckInStr = rows[i].CheckInAt.Format("15:04")
			}
		}

		sort.Slice(rows, func(i, j int) bool {
			if !rows[i].ClassDate.Equal(rows[j].ClassDate) {
				return rows[i].ClassDate.Before(rows[j].ClassDate)
			}
			if rows[i].ClassName != rows[j].ClassName {
				return rows[i].ClassName < rows[j].ClassName
			}
			if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
				return rows[i].CreatedAt.Before(rows[j].CreatedAt)
			}
			return statusWeight(rows[i].Status) < statusWeight(rows[j].Status)
		})

		ranks := map[uint]int{}
		for i := range rows {
			if rows[i].Status == "waitlisted" {
				ranks[rows[i].ClassID]++
				rows[i].WaitlistRank = ranks[rows[i].ClassID]
			}
		}

		answers := map[uint][]string{}
		if len(rows) > 0 {
			regIDs := make([]uint, 0, len(rows))
			for _, rr := range rows { regIDs = append(regIDs, rr.ID) }

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
					if strings.TrimSpace(a.Answer) == "" { continue }
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
			},
			HasResult: len(rows) > 0,
			Answers:   answers,
		}

		view, err := t.Clone()
		if err != nil { http.Error(w, err.Error(), 500); return }
		if _, err := view.ParseFiles("templates/pages/admin/roster.tmpl"); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		if err := view.ExecuteTemplate(w, "admin/roster.tmpl", vm); err != nil {
			http.Error(w, err.Error(), 500); return
		}
	}
}


// ---------- Admin Roster CSV ----------
func AdminRosterCSV(w http.ResponseWriter, r *http.Request) {
	fFrom := r.URL.Query().Get("from")
	fTo := r.URL.Query().Get("to")
	fClassID := r.URL.Query().Get("class_id")
	fStatus := r.URL.Query().Get("status")

	now := time.Now()
	from := parseDate(fFrom, now.AddDate(0, 0, -30))
	to := parseDate(fTo, now.AddDate(0, 0, 30))

	q := db.Conn().Table("registrations").
		Select(`registrations.id, registrations.code, registrations.status, registrations.check_in_at, registrations.created_at,
		        registrations.parent_id as parent_id,
		        children.name as child_name,
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
		case "confirmed", "waitlisted", "canceled":
			q = q.Where("registrations.status = ?", fStatus)
		case "checked-in":
			q = q.Where("registrations.check_in_at IS NOT NULL")
		}
	}

	var rows []rosterRow
	if err := q.Scan(&rows).Error; err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("roster-%s.csv", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"Date", "Class", "Child", "Gender", "DOB", "Parent", "Phone", "Code", "Status", "CheckedInAt"})
	for _, row := range rows {
		dateStr := row.ClassDate.Format("2006-01-02") // date-only in CSV
		checkStr := ""
		if row.CheckInAt != nil {
			checkStr = row.CheckInAt.Format("2006-01-02 15:04")
		}
		dobStr := ""
		if !row.BirthDate.IsZero() {
		    dobStr = row.BirthDate.Format("2006-01-02")
		}
		_ = cw.Write([]string{
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
		})
	}
}
