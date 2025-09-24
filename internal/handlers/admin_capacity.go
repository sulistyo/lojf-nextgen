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

func parseDateCap(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return def
	}
	return t
}

func AdminCapacity(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		now := time.Now()
		from := parseDateCap(fromStr, now.AddDate(0, 0, -30))
		to := parseDateCap(toStr, now.AddDate(0, 0, 30))

		// Load classes in window
		var classes []models.Class
		if err := db.Conn().
			Where("date BETWEEN ? AND ?", from, to).
			Order("date asc, name asc").
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
				Where("class_id = ? AND status = ?", c.ID, "confirmed").
				Count(&confirmed)

			var waitlisted int64
			db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND status = ?", c.ID, "waitlisted").
				Count(&waitlisted)

			var checkedIn int64
			db.Conn().Model(&models.Registration{}).
				Where("class_id = ? AND check_in_at IS NOT NULL", c.ID).
				Count(&checkedIn)

			avail := c.Capacity - int(confirmed)
			if avail < 0 {
				avail = 0
			}
			fill := 0
			if c.Capacity > 0 {
				fill = int(confirmed * 100 / int64(c.Capacity))
			}

			rows = append(rows, capacityRow{
				ClassID:     c.ID,
				ClassName:   c.Name,
				ClassDate:   c.Date,
				DateStr:     c.Date.Format("Mon, 02 Jan 2006 15:04"),
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
			From:  from.Format("2006-01-02"),
			To:    to.Format("2006-01-02"),
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
