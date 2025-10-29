// internal/handlers/cookies.go (or wherever set/read live)
package handlers

import (
	"net/http"
	"time"
)

func clearParentCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    "parent_phone",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "parent_name",
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})
}

// GET /switch-number?return=/register
func SwitchNumber(w http.ResponseWriter, r *http.Request) {
	clearParentCookies(w)
	ret := r.URL.Query().Get("return")
	if ret == "" {
		ret = "/register"
	}
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
