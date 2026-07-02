package handlers

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type attendanceRow struct {
	Rank        int
	ChildID     uint
	ChildName   string
	ParentName  string
	ParentPhone string
	Attended    int64 // distinct class sessions checked-in within the period
	LastStr     string
}

// checkInDateStr formats a raw check_in_at string (as stored by SQLite, e.g.
// "2026-06-27 17:16:21.61+07:00") into the dd-mm-yyyy Jakarta date used across
// the admin UI. Falls back to the leading date portion if the layout differs.
func checkInDateStr(raw string) string {
	if raw == "" {
		return ""
	}
	if ts, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", raw); err == nil {
		return fmtDate(ts)
	}
	if len(raw) >= 10 {
		if d, err := time.Parse("2006-01-02", raw[:10]); err == nil {
			return fmtDate(d)
		}
		return raw[:10]
	}
	return raw
}

type attendanceVM struct {
	Title      string
	Rows       []attendanceRow
	From       string
	To         string
	Class      string   // selected class-name filter ("" = all)
	ClassNames []string // distinct class names for the dropdown
	Summary    struct {
		Students    int
		Attendances int64
	}
}

// distinctClassNames returns the sorted set of class names for the filter dropdown.
func distinctClassNames() []string {
	var names []string
	_ = db.Conn().Model(&models.Class{}).
		Distinct("name").
		Order("name asc").
		Pluck("name", &names).Error
	return names
}

// attendanceWindow parses from/to (YYYY-MM-DD) as Jakarta calendar days and
// returns an inclusive UTC window [start-of-from-day, end-of-to-day] for
// filtering classes.date. Defaults to the last 90 days through today.
func attendanceWindow(fFrom, fTo string) (fromUTC, toUTC time.Time, fromStr, toStr string) {
	loc := capacityLoc
	nowJ := time.Now().In(loc)
	today := time.Date(nowJ.Year(), nowJ.Month(), nowJ.Day(), 0, 0, 0, 0, loc)
	fromJ := parseDateCapJKT(fFrom, today.AddDate(0, 0, -90), loc)
	toJ := parseDateCapJKT(fTo, today, loc)
	fromUTC = time.Date(fromJ.Year(), fromJ.Month(), fromJ.Day(), 0, 0, 0, 0, loc).UTC()
	toUTC = time.Date(toJ.Year(), toJ.Month(), toJ.Day(), 23, 59, 59, 0, loc).UTC()
	return fromUTC, toUTC, fromJ.Format("2006-01-02"), toJ.Format("2006-01-02")
}

// queryAttendance returns per-child attendance counts (check-ins) for classes
// whose date falls inside [fromUTC, toUTC], sorted by most frequent first.
func queryAttendance(fromUTC, toUTC time.Time, className string) ([]attendanceRow, error) {
	type attAgg struct {
		ChildID     uint
		ChildName   string
		ParentName  string
		ParentPhone string
		Attended    int64
		LastAt      string
	}
	q := db.Conn().Table("registrations").
		Select(`children.id AS child_id, children.name AS child_name,
			parents.name AS parent_name, parents.phone AS parent_phone,
			COUNT(DISTINCT registrations.class_id) AS attended,
			MAX(registrations.check_in_at) AS last_at`).
		Joins("JOIN children ON children.id = registrations.child_id").
		Joins("JOIN classes  ON classes.id  = registrations.class_id").
		Joins("JOIN parents  ON parents.id  = registrations.parent_id").
		Where("registrations.check_in_at IS NOT NULL").
		Where("classes.date BETWEEN ? AND ?", fromUTC, toUTC)

	if className != "" {
		q = q.Where("classes.name = ?", className)
	}

	var aggs []attAgg
	err := q.Group("children.id").
		Order("attended DESC, child_name ASC").
		Scan(&aggs).Error
	if err != nil {
		return nil, err
	}

	rows := make([]attendanceRow, 0, len(aggs))
	for i, a := range aggs {
		rows = append(rows, attendanceRow{
			Rank:        i + 1,
			ChildID:     a.ChildID,
			ChildName:   a.ChildName,
			ParentName:  a.ParentName,
			ParentPhone: a.ParentPhone,
			Attended:    a.Attended,
			LastStr:     checkInDateStr(a.LastAt),
		})
	}
	return rows, nil
}

// GET /admin/attendance
func AdminAttendance(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/admin/attendance.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		fromUTC, toUTC, fromStr, toStr := attendanceWindow(
			r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		className := r.URL.Query().Get("class")

		rows, err := queryAttendance(fromUTC, toUTC, className)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		vm := attendanceVM{
			Title: "Admin • Attendance", Rows: rows, From: fromStr, To: toStr,
			Class: className, ClassNames: distinctClassNames(),
		}
		vm.Summary.Students = len(rows)
		for _, rr := range rows {
			vm.Summary.Attendances += rr.Attended
		}

		if err := view.ExecuteTemplate(w, "admin/attendance.tmpl", vm); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
}

// GET /admin/attendance.csv
func AdminAttendanceCSV(w http.ResponseWriter, r *http.Request) {
	fromUTC, toUTC, fromStr, toStr := attendanceWindow(
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	className := r.URL.Query().Get("class")

	rows, err := queryAttendance(fromUTC, toUTC, className)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="attendance_%s_to_%s.csv"`, fromStr, toStr))

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"Rank", "Child", "Parent", "Phone", "TimesAttended", "LastAttended"})
	for _, rr := range rows {
		_ = cw.Write([]string{
			strconv.Itoa(rr.Rank),
			rr.ChildName,
			rr.ParentName,
			rr.ParentPhone,
			strconv.FormatInt(rr.Attended, 10),
			rr.LastStr,
		})
	}
}
