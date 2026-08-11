package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

const (
	adminCookieName = "admin_session"
	staffCookieName = "checkin_staff"

	// Admins get a short session; a shared check-in station gets a long one so
	// volunteers never type a password (an admin signs the tablet in once).
	adminSessionTTL   = 12 * time.Hour
	checkinSessionTTL = 30 * 24 * time.Hour
)

// ---------- session secret ----------

var (
	secretOnce sync.Once
	secretVal  []byte
)

// sessionSecret returns the HMAC key for session cookies. SESSION_SECRET wins;
// otherwise a random key is generated once and persisted in app_settings so
// restarts do not sign everyone out.
func sessionSecret() []byte {
	secretOnce.Do(func() {
		if s := os.Getenv("SESSION_SECRET"); s != "" {
			secretVal = []byte(s)
			return
		}
		var row models.AppSetting
		if err := db.Conn().Where("key = ?", "session_secret").First(&row).Error; err == nil && row.Value != "" {
			if b, err := hex.DecodeString(row.Value); err == nil && len(b) >= 32 {
				secretVal = b
				return
			}
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			log.Fatalf("session secret: %v", err)
		}
		db.Conn().Save(&models.AppSetting{Key: "session_secret", Value: hex.EncodeToString(buf), UpdatedAt: time.Now()})
		secretVal = buf
	})
	return secretVal
}

// signToken builds "<userID>.<expUnix>.<hmac>".
func signToken(userID uint, exp time.Time) string {
	body := fmt.Sprintf("%d.%d", userID, exp.Unix())
	mac := hmac.New(sha256.New, sessionSecret())
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// parseToken verifies the signature and expiry, returning the user ID.
func parseToken(tok string) (uint, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return 0, false
	}
	body := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, sessionSecret())
	mac.Write([]byte(body))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) != 1 {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().After(time.Unix(exp, 0)) {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// ---------- current user ----------

type ctxKey string

const ctxUserKey ctxKey = "adminUser"

// CurrentUser returns the logged-in account, or nil.
func CurrentUser(r *http.Request) *models.AdminUser {
	if u, ok := r.Context().Value(ctxUserKey).(*models.AdminUser); ok {
		return u
	}
	return nil
}

// StaffName returns the volunteer name declared for this shift, if any.
func StaffName(r *http.Request) string {
	c, err := r.Cookie(staffCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

// actorLabel is who to blame in the audit log: the shift name when a shared
// check-in account is in use, otherwise the username.
func actorLabel(r *http.Request) string {
	if s := StaffName(r); s != "" {
		return s
	}
	if u := CurrentUser(r); u != nil {
		return u.Username
	}
	return ""
}

// ---------- middleware ----------

// RequireRole blocks the request unless the caller is logged in with one of the
// given roles. Server-side enforcement is the real gate; hiding buttons in
// templates is cosmetic only.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(adminCookieName)
			if err != nil || c.Value == "" {
				redirectLogin(w, r)
				return
			}
			id, ok := parseToken(c.Value)
			if !ok {
				clearAdminCookie(w)
				redirectLogin(w, r)
				return
			}
			var u models.AdminUser
			if err := db.Conn().First(&u, id).Error; err != nil || !u.Active {
				clearAdminCookie(w)
				redirectLogin(w, r)
				return
			}
			allowed := false
			for _, want := range roles {
				if u.Role == want {
					allowed = true
					break
				}
			}
			if !allowed {
				// Give volunteers a way back instead of a dead-end error page.
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `<!doctype html><meta charset="utf-8">`+
					`<div style="font-family:system-ui;max-width:32rem;margin:4rem auto;padding:0 1rem">`+
					`<h1 style="font-size:1.25rem">Tidak punya akses</h1>`+
					`<p>Akun <strong>%s</strong> (%s) tidak boleh membuka halaman ini.</p>`+
					`<p><a href="%s">← Kembali</a></p></div>`,
					template.HTMLEscapeString(u.Username),
					template.HTMLEscapeString(u.Role),
					defaultLanding(u.Role))
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserKey, &u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is kept as a thin alias so existing wiring stays readable.
func RequireAdmin(next http.Handler) http.Handler {
	return RequireRole(models.RoleAdmin)(next)
}

func redirectLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/login?next="+r.URL.RequestURI(), http.StatusSeeOther)
}

func clearAdminCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: "", Path: "/", HttpOnly: true, Expires: time.Unix(0, 0),
	})
}

// isHTTPS reports whether the original client request used TLS, so the Secure
// flag is set in production but local http:// development still works.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// ---------- login rate limiting ----------

type loginAttempt struct {
	n     int
	until time.Time
}

var (
	attemptsMu sync.Mutex
	attempts   = map[string]*loginAttempt{}
)

const (
	maxLoginFails = 8
	lockoutFor    = 15 * time.Minute
)

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loginLocked(ip string) bool {
	attemptsMu.Lock()
	defer attemptsMu.Unlock()
	a := attempts[ip]
	return a != nil && a.n >= maxLoginFails && time.Now().Before(a.until)
}

func noteLoginFail(ip string) {
	attemptsMu.Lock()
	defer attemptsMu.Unlock()
	a := attempts[ip]
	if a == nil || time.Now().After(a.until) {
		a = &loginAttempt{}
		attempts[ip] = a
	}
	a.n++
	a.until = time.Now().Add(lockoutFor)
}

func clearLoginFails(ip string) {
	attemptsMu.Lock()
	defer attemptsMu.Unlock()
	delete(attempts, ip)
}

// ---------- handlers ----------

// GET /admin/login
func AdminLoginForm(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/admin/login.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		if err := view.ExecuteTemplate(w, "admin/login.tmpl", map[string]any{
			"Title": "Admin • Login",
			"Next":  r.URL.Query().Get("next"),
			"Error": r.URL.Query().Get("error"),
		}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

// POST /admin/login
func AdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ip := clientIP(r)
	if loginLocked(ip) {
		http.Redirect(w, r, "/admin/login?error=locked", http.StatusSeeOther)
		return
	}

	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	pw := r.FormValue("password")
	next := r.FormValue("next")

	var u models.AdminUser
	err := db.Conn().Where("username = ?", username).First(&u).Error

	// Always run a bcrypt comparison so a missing username and a wrong password
	// take the same time and cannot be told apart.
	hash := u.PassHash
	if err != nil || hash == "" {
		hash = "$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalidin"
	}
	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))

	if err != nil || bcryptErr != nil || !u.Active {
		noteLoginFail(ip)
		log.Printf("admin login failed: user=%q ip=%s", username, ip)
		http.Redirect(w, r, "/admin/login?error=invalid", http.StatusSeeOther)
		return
	}
	clearLoginFails(ip)

	ttl := adminSessionTTL
	if u.Role == models.RoleCheckin {
		ttl = checkinSessionTTL
	}
	exp := time.Now().Add(ttl)
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    signToken(u.ID, exp),
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  exp,
	})

	now := time.Now()
	db.Conn().Model(&u).Update("last_login", &now)
	writeAudit(r, &u, "auth.login", "user:"+u.Username, "")

	if next == "" || !strings.HasPrefix(next, "/") {
		next = defaultLanding(u.Role)
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func defaultLanding(role string) string {
	if role == models.RoleCheckin {
		return "/station"
	}
	return "/admin/classes"
}

// POST /admin/logout
func AdminLogout(w http.ResponseWriter, r *http.Request) {
	clearAdminCookie(w)
	http.SetCookie(w, &http.Cookie{
		Name: staffCookieName, Value: "", Path: "/", HttpOnly: true, Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// ---------- bootstrap ----------

// EnsureBootstrapAdmin creates a first admin account when the table is empty so
// a fresh install is reachable. The password comes from ADMIN_PASSWORD; if that
// is unset a random one is generated and logged once.
func EnsureBootstrapAdmin(gdb *gorm.DB) error {
	var n int64
	if err := gdb.Model(&models.AdminUser{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	pw := os.Getenv("ADMIN_PASSWORD")
	generated := false
	if pw == "" {
		buf := make([]byte, 12)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		pw = base64.RawURLEncoding.EncodeToString(buf)
		generated = true
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := models.AdminUser{Username: "admin", PassHash: string(hash), Role: models.RoleAdmin, Active: true}
	if err := gdb.Create(&u).Error; err != nil {
		return err
	}
	if generated {
		log.Printf("BOOTSTRAP: created admin user %q with generated password: %s", u.Username, pw)
		log.Printf("BOOTSTRAP: log in and change it at /admin/users immediately")
	} else {
		log.Printf("BOOTSTRAP: created admin user %q from ADMIN_PASSWORD", u.Username)
	}
	return nil
}
