package handlers

import (
	"html/template"
	"net/http"
	"os"
	"time"
)

const adminCookieName = "admin_session"

// Default password if env not set
func adminPassword() string {
	if p := os.Getenv("ADMIN_PASSWORD"); p != "" {
		return p
	}
	return "admin123" // change in production: export ADMIN_PASSWORD=...
}

// RequireAdmin is middleware: blocks access unless logged in
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(adminCookieName)
		if err != nil || c.Value != "ok" {
			http.Redirect(w, r, "/admin/login?next="+r.URL.RequestURI(), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /admin/login
func AdminLoginForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := t.Clone()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := view.ParseFiles("templates/pages/admin/login.tmpl"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		_ = view.ExecuteTemplate(w, "admin/login.tmpl", map[string]any{
			"Title": "Admin • Login",
			"Next":  r.URL.Query().Get("next"),
		})
	}
}

// POST /admin/login
func AdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	pw := r.FormValue("password")
	next := r.FormValue("next")
	if pw != adminPassword() {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "ok",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	if next == "" {
		next = "/admin/classes"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// POST /admin/logout
func AdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
