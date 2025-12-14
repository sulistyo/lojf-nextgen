package handlers

import (
	"html/template"
	"net/http"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type capacityRow struct {
	ClassID     uint
	ClassName   string
	ClassDate   time.Time
	DateStr     string
	Capacity    int
	Confirmed   int64
	Waitlisted  int64
	CheckedIn   int64
	Available   int
	FillPercent int
}

type capacityVM struct {
	Title   string
	Rows    []capacityRow
	From    string
	To      string
	Summary struct {
		Classes    int
		Capacity   int
		Confirmed  int64
		Waitlisted int64
		CheckedIn  int64
	}
}

func parseDateCapJKT(s string, def time.Time, loc *time.Location) time.Time {
	if s == "" {
		return def
	}
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return def
	}
	return t
}

func AdminCapacity(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loc, _ := time.LoadLocation("Asia/Jakarta")
		if loc == nil {
			loc = time.FixedZone("WIB", 7*3600)
		}

		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")

		// Use Jakarta day boundaries so it matches roster + parent views regardless of server TZ.
		nowJ := time.Now().In(loc)
		defFrom := time.Date(nowJ.Year(), nowJ.Month(), nowJ.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -30)
		defTo := time.Date(nowJ.Year(), nowJ.Month(), nowJ.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 30)

		fromJ := parseDateCapJKT(fromStr, defFrom, loc)
		toJ := parseDateCapJKT(toStr, defTo, loc)

		// Convert to UTC window for DB compare (DB stores UTC timestamps).
		fromUTC := time.Date(fromJ.Year(), fromJ.Month(), fromJ.Day(), 0, 0, 0, 0, loc).UTC()
		// inclusive end-of-day
		toUTC := time.Date(toJ.Year(), toJ.Month(), toJ.Day(), 23, 59, 59, 0, loc).UTC()

		// Load classes in window
		var classes []models.Class
		if err := db.Conn().
			Where("date BETWEEN ? AND ?", fromUTC, toUTC).
			Order("date desc, name asc").
			Find(&classes).Error; err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		rows := make([]capacityRow, 0, len(classes))
		var totalCap int
		var totalConf, totalWait, totalIn int64

		for _, c := range classes {
			var confirmed int64
			db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ? AND check_in_at IS NULL", c.ID, "confirmed").
				Count(&confirmed)

			var waitlisted int64
			db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ?", c.ID, "waitlisted").
				Count(&waitlisted)

			var checkedIn int64
			db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ? AND check_in_at IS NOT NULL", c.ID, "confirmed").
				Count(&checkedIn)


			avail := c.Capacity - int(confirmed) - int(checkedIn)
			if avail < 0 {
				avail = 0
			}
			fill := 0
			if c.Capacity > 0 {
				fill = int(confirmed + checkedIn * 100 / int64(c.Capacity))
			}

			// IMPORTANT: use the same formatting helper as roster so day/time doesn't drift with server TZ.
			rows = append(rows, capacityRow{
				ClassID:     c.ID,
				ClassName:   c.Name,
				ClassDate:   c.Date,
				DateStr:     fmtDate(c.Date),
				Capacity:    c.Capacity,
				Confirmed:   confirmed,
				Waitlisted:  waitlisted,
				CheckedIn:   checkedIn,
				Available:   avail,
				FillPercent: fill,
			})

			totalCap += c.Capacity
			totalConf += confirmed
			totalWait += waitlisted
			totalIn += checkedIn
		}

		vm := capacityVM{
			Title: "Admin • Capacity",
			Rows:  rows,
			From:  fromJ.Format("2006-01-02"),
			To:    toJ.Format("2006-01-02"),
		}
		vm.Summary.Classes = len(classes)
		vm.Summary.Capacity = totalCap
		vm.Summary.Confirmed = totalConf
		vm.Summary.Waitlisted = totalWait
		vm.Summary.CheckedIn = totalIn

		view, err := t.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := view.ParseFiles("templates/pages/admin/capacity.tmpl"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := view.ExecuteTemplate(w, "admin/capacity.tmpl", vm); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
}
