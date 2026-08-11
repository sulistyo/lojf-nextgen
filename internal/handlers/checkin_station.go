package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// todayWindow returns the [start, end] of the current Jakarta day in UTC.
// Class dates are stored at Jakarta midnight, so a naive UTC comparison drops
// same-day classes — the bug fixed in dbfd111 for the roster.
func todayWindow() (time.Time, time.Time) {
	nowJ := time.Now().In(rosterLoc)
	start := time.Date(nowJ.Year(), nowJ.Month(), nowJ.Day(), 0, 0, 0, 0, rosterLoc)
	end := time.Date(nowJ.Year(), nowJ.Month(), nowJ.Day(), 23, 59, 59, 0, rosterLoc)
	return start.UTC(), end.UTC()
}

// ErrCheckinNotToday / ErrCheckinWrongCampus are the two guards that stop a
// check-in volunteer from marking attendance outside their shift.
var (
	ErrCheckinNotToday    = errors.New("kelas ini bukan hari ini")
	ErrCheckinWrongCampus = errors.New("kelas ini bukan campus akun ini")
)

// guardCheckin enforces the check-in role's boundaries. Admins bypass both
// checks so they can still fix records after the fact.
//
// This is what would have rejected the 2026-08-07 incident: two registrations
// for a class dated 2026-08-08, checked in a day early.
func guardCheckin(u *models.AdminUser, class models.Class) error {
	if u == nil || u.Role == models.RoleAdmin {
		return nil
	}
	if !campusAllows(u.Campus, class.Name) {
		return ErrCheckinWrongCampus
	}
	start, end := todayWindow()
	d := class.Date.UTC()
	if d.Before(start) || d.After(end) {
		return ErrCheckinNotToday
	}
	return nil
}

type stationKid struct {
	RegID       uint
	Code        string
	ChildName   string
	CheckInAt   *time.Time
	CheckedInBy string
	TimeStr     string
}

type stationClass struct {
	ClassID  uint
	Name     string
	Total    int
	Checked  int
	Kids     []stationKid
}

type stationVM struct {
	Title     string
	StaffName string
	Campus    string
	DateStr   string
	Classes   []stationClass
	Flash     *Flash
	Username  string
}

// GET /station — the volunteer check-in screen.
func CheckinStation(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/admin/station.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		u := CurrentUser(r)
		staff := StaffName(r)

		start, end := todayWindow()

		type row struct {
			RegID       uint
			Code        string
			Status      string
			CheckInAt   *time.Time
			CheckedInBy string
			ChildName   string
			ClassID     uint
			ClassName   string
		}
		var rows []row
		q := db.Conn().Table("registrations").
			Select(`registrations.id AS reg_id,
			        registrations.code AS code,
			        registrations.status AS status,
			        registrations.check_in_at AS check_in_at,
			        registrations.checked_in_by AS checked_in_by,
			        children.name AS child_name,
			        classes.id AS class_id,
			        classes.name AS class_name`).
			Joins("JOIN children ON children.id = registrations.child_id").
			Joins("JOIN classes  ON classes.id  = registrations.class_id").
			Where("registrations.status = ?", "confirmed").
			Where("classes.date BETWEEN ? AND ?", start, end).
			Order("classes.name ASC, children.name ASC")
		if err := q.Scan(&rows).Error; err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		byClass := map[uint]*stationClass{}
		var order []uint
		for _, rw := range rows {
			if u != nil && !campusAllows(u.Campus, rw.ClassName) {
				continue
			}
			sc := byClass[rw.ClassID]
			if sc == nil {
				sc = &stationClass{ClassID: rw.ClassID, Name: rw.ClassName}
				byClass[rw.ClassID] = sc
				order = append(order, rw.ClassID)
			}
			k := stationKid{
				RegID:       rw.RegID,
				Code:        rw.Code,
				ChildName:   rw.ChildName,
				CheckInAt:   rw.CheckInAt,
				CheckedInBy: rw.CheckedInBy,
			}
			if rw.CheckInAt != nil {
				k.TimeStr = rw.CheckInAt.In(rosterLoc).Format("15:04")
				sc.Checked++
			}
			sc.Total++
			sc.Kids = append(sc.Kids, k)
		}

		classes := make([]stationClass, 0, len(order))
		for _, id := range order {
			classes = append(classes, *byClass[id])
		}

		campus := ""
		username := ""
		if u != nil {
			campus = u.Campus
			username = u.Username
			if campus == "" {
				campus = "Semua campus"
			}
		}

		if err := view.ExecuteTemplate(w, "admin/station.tmpl", stationVM{
			Title:     "Check-in",
			StaffName: staff,
			Campus:    campus,
			Username:  username,
			DateStr:   time.Now().In(rosterLoc).Format("Mon, 02 Jan 2006"),
			Classes:   classes,
			Flash:     MakeFlash(r, r.URL.Query().Get("error"), r.URL.Query().Get("ok")),
		}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

// POST /station/staff — record who is on duty for this shift.
func CheckinStationStaff(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	name := strings.TrimSpace(r.FormValue("staff_name"))
	if len(name) > 60 {
		name = name[:60]
	}
	http.SetCookie(w, &http.Cookie{
		Name:     staffCookieName,
		Value:    name,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(16 * time.Hour),
	})
	http.Redirect(w, r, "/station", http.StatusSeeOther)
}
