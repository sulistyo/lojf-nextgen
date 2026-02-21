package handlers

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type familyRow struct {
	ParentID     uint
	Name         string
	Phone        string
	Children     string // "Alice (8), Bob (6)"
	Sessions     int    // distinct classes in period
	IsNew        bool
	FirstDateStr string
}

type familiesVM struct {
	Title             string
	From              string
	To                string
	Rows              []familyRow
	TotalFamilies     int
	NewFamilies       int
	ReturningFamilies int
	PctNew            int
}

func AdminFamilies(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/admin/families.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		loc, _ := time.LoadLocation("Asia/Jakarta")
		now := time.Now().In(loc)

		// Default: current month
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")

		var from, to time.Time

		if fromStr == "" {
			from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
			fromStr = from.Format("2006-01-02")
		} else {
			var err error
			from, err = time.ParseInLocation("2006-01-02", fromStr, loc)
			if err != nil {
				http.Error(w, "invalid from date", 400)
				return
			}
		}

		if toStr == "" {
			// last day of current month
			to = time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, loc)
			toStr = to.Format("2006-01-02")
		} else {
			var err error
			to, err = time.ParseInLocation("2006-01-02", toStr, loc)
			if err != nil {
				http.Error(w, "invalid to date", 400)
				return
			}
		}

		// inclusive upper bound: end of day in Jakarta
		toEnd := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, loc)

		// 1. Unique parent IDs with confirmed registrations in the period
		var activeParentIDs []uint
		if err := db.Conn().
			Table("registrations").
			Joins("JOIN classes ON classes.id = registrations.class_id").
			Where("registrations.status = 'confirmed' AND classes.date >= ? AND classes.date <= ?", from.UTC(), toEnd.UTC()).
			Distinct().
			Pluck("registrations.parent_id", &activeParentIDs).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}

		if len(activeParentIDs) == 0 {
			_ = view.ExecuteTemplate(w, "admin/families.tmpl", familiesVM{
				Title: "Admin • Families",
				From:  fromStr,
				To:    toStr,
			})
			return
		}

		// 2. Load parent records
		var parents []models.Parent
		if err := db.Conn().Where("id IN ?", activeParentIDs).Find(&parents).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}

		// 3. First-ever confirmed class date per parent (to classify new vs returning)
		// NOTE: MIN() on a date column returns a raw string in SQLite; scan as string then parse.
		type firstClassRow struct {
			ParentID       uint
			FirstClassDate string `gorm:"column:first_class_date"`
		}
		var firsts []firstClassRow
		if err := db.Conn().
			Table("registrations").
			Select("registrations.parent_id, MIN(classes.date) as first_class_date").
			Joins("JOIN classes ON classes.id = registrations.class_id").
			Where("registrations.parent_id IN ? AND registrations.status = 'confirmed'", activeParentIDs).
			Group("registrations.parent_id").
			Scan(&firsts).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}
		firstMap := make(map[uint]time.Time, len(firsts))
		for _, f := range firsts {
			for _, layout := range []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02 15:04:05.999999999-07:00",
				"2006-01-02 15:04:05-07:00",
				"2006-01-02 15:04:05+00:00",
				"2006-01-02 15:04:05",
				"2006-01-02",
			} {
				if t, err := time.Parse(layout, f.FirstClassDate); err == nil {
					firstMap[f.ParentID] = t
					break
				}
			}
		}

		// 4. Distinct session count per parent in period
		type sessionCountRow struct {
			ParentID uint
			Sessions int
		}
		var sessionCounts []sessionCountRow
		if err := db.Conn().
			Table("registrations").
			Select("registrations.parent_id, COUNT(DISTINCT registrations.class_id) as sessions").
			Joins("JOIN classes ON classes.id = registrations.class_id").
			Where("registrations.parent_id IN ? AND registrations.status = 'confirmed' AND classes.date >= ? AND classes.date <= ?",
				activeParentIDs, from.UTC(), toEnd.UTC()).
			Group("registrations.parent_id").
			Scan(&sessionCounts).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}
		sessionMap := make(map[uint]int, len(sessionCounts))
		for _, s := range sessionCounts {
			sessionMap[s.ParentID] = s.Sessions
		}

		// 5. Unique children per parent in the period (for display)
		type kidInPeriod struct {
			ParentID  uint
			ChildName string
			BirthDate time.Time
		}
		var kidRows []kidInPeriod
		if err := db.Conn().
			Table("registrations").
			Select("registrations.parent_id, children.name as child_name, children.birth_date").
			Joins("JOIN classes ON classes.id = registrations.class_id").
			Joins("JOIN children ON children.id = registrations.child_id").
			Where("registrations.parent_id IN ? AND registrations.status = 'confirmed' AND classes.date >= ? AND classes.date <= ?",
				activeParentIDs, from.UTC(), toEnd.UTC()).
			Group("registrations.parent_id, registrations.child_id").
			Scan(&kidRows).Error; err != nil {
			http.Error(w, "db error", 500)
			return
		}

		// Group children by parent
		type kidInfo struct {
			Name      string
			BirthDate time.Time
		}
		kidMap := make(map[uint][]kidInfo, len(activeParentIDs))
		for _, k := range kidRows {
			kidMap[k.ParentID] = append(kidMap[k.ParentID], kidInfo{k.ChildName, k.BirthDate})
		}

		ageStr := func(dob time.Time) string {
			if dob.IsZero() {
				return ""
			}
			n := time.Now().In(loc)
			y := n.Year() - dob.In(loc).Year()
			if n.Before(time.Date(n.Year(), dob.In(loc).Month(), dob.In(loc).Day(), 0, 0, 0, 0, loc)) {
				y--
			}
			if y < 0 {
				y = 0
			}
			return strconv.Itoa(y)
		}

		// Build rows
		rows := make([]familyRow, 0, len(parents))
		newCount := 0

		for _, p := range parents {
			firstDate := firstMap[p.ID]
			// "New" = their very first confirmed class falls within this period
			isNew := !firstDate.Before(from.UTC()) && !firstDate.After(toEnd.UTC())

			// Build children label
			kids := kidMap[p.ID]
			parts := make([]string, 0, len(kids))
			for _, k := range kids {
				age := ageStr(k.BirthDate)
				if age != "" {
					parts = append(parts, k.Name+" ("+age+")")
				} else {
					parts = append(parts, k.Name)
				}
			}

			row := familyRow{
				ParentID:     p.ID,
				Name:         p.Name,
				Phone:        p.Phone,
				Children:     strings.Join(parts, ", "),
				Sessions:     sessionMap[p.ID],
				IsNew:        isNew,
				FirstDateStr: firstDate.In(loc).Format("02 Jan 2006"),
			}
			if isNew {
				newCount++
			}
			rows = append(rows, row)
		}

		// Sort: new families first, then alphabetically by name
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].IsNew != rows[j].IsNew {
				return rows[i].IsNew
			}
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})

		total := len(rows)
		returning := total - newCount
		pctNew := 0
		if total > 0 {
			pctNew = newCount * 100 / total
		}

		vm := familiesVM{
			Title:             "Admin • Families",
			From:              fromStr,
			To:                toStr,
			Rows:              rows,
			TotalFamilies:     total,
			NewFamilies:       newCount,
			ReturningFamilies: returning,
			PctNew:            pctNew,
		}
		if err := view.ExecuteTemplate(w, "admin/families.tmpl", vm); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}
