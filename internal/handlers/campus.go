package handlers

import "strings"

// CampusOf derives the campus code from a class name.
//
// Class names are consistently prefixed with the campus:
//
//	"FJB Little Stars (Feast Jakarta Barat)"  -> "FJB"
//	"FJB - Stars Club (Feast Jakarta Barat)"  -> "FJB"
//	"FJU Awesome Kids (Feast Jakarta Utara)"  -> "FJU"
//
// Taking the first whitespace-separated token survives the "FJB - ..." variant.
// If classes ever gain a real campus column, this is the single place to change.
func CampusOf(className string) string {
	f := strings.Fields(strings.TrimSpace(className))
	if len(f) == 0 {
		return ""
	}
	return strings.ToUpper(f[0])
}

// campusAllows reports whether an account scoped to userCampus may act on a
// class named className. An empty userCampus means "all campuses".
func campusAllows(userCampus, className string) bool {
	if strings.TrimSpace(userCampus) == "" {
		return true
	}
	return strings.EqualFold(userCampus, CampusOf(className))
}
