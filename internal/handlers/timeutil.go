package handlers

import "time"

// Asia/Jakarta for all display formatting
var tzJakarta *time.Location

func init() {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// fallback to UTC if the tzdata is missing (unlikely on Ubuntu)
		tzJakarta = time.UTC
		return
	}
	tzJakarta = loc
}

// Date-only friendly string, e.g. "Mon, 02 Jan 2006"
func fmtDate(d time.Time) string {
	return d.In(tzJakarta).Format("Mon, 02 Jan 2006")
}

// ISO date string, e.g. "2006-01-02"
func fmtISODate(d time.Time) string {
	return d.In(tzJakarta).Format("2006-01-02")
}
