package handlers

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

type userRow struct {
	models.AdminUser
	LastLoginStr string
}

type usersVM struct {
	Title   string
	Users   []userRow
	Me      string
	Flash   *Flash
	Roles   []string
	Campus  []string
}

// GET /admin/users
func AdminUsers(t *template.Template) http.HandlerFunc {
	view := template.Must(t.Clone())
	template.Must(view.ParseFiles("templates/pages/admin/users.tmpl"))

	return func(w http.ResponseWriter, r *http.Request) {
		var us []models.AdminUser
		if err := db.Conn().Order("role ASC, username ASC").Find(&us).Error; err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		rows := make([]userRow, 0, len(us))
		for _, u := range us {
			row := userRow{AdminUser: u, LastLoginStr: "belum pernah"}
			if u.LastLogin != nil {
				row.LastLoginStr = u.LastLogin.In(rosterLoc).Format("02 Jan 2006 15:04")
			}
			rows = append(rows, row)
		}
		me := ""
		if cu := CurrentUser(r); cu != nil {
			me = cu.Username
		}
		if err := view.ExecuteTemplate(w, "admin/users.tmpl", usersVM{
			Title:  "Admin • Akun",
			Users:  rows,
			Me:     me,
			Roles:  []string{models.RoleAdmin, models.RoleCheckin},
			Campus: []string{"", "FJB", "FJU"},
			Flash:  MakeFlash(r, r.URL.Query().Get("error"), r.URL.Query().Get("ok")),
		}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}

// POST /admin/users
func AdminUserCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	pw := r.FormValue("password")
	role := r.FormValue("role")
	campus := strings.ToUpper(strings.TrimSpace(r.FormValue("campus")))

	if username == "" || len(pw) < 8 {
		http.Redirect(w, r, "/admin/users?error=username+wajib+dan+password+minimal+8+karakter", http.StatusSeeOther)
		return
	}
	if role != models.RoleAdmin && role != models.RoleCheckin {
		http.Redirect(w, r, "/admin/users?error=role+tidak+valid", http.StatusSeeOther)
		return
	}
	// Campus only constrains check-in accounts; admins always see everything.
	if role == models.RoleAdmin {
		campus = ""
	}
	var n int64
	db.Conn().Model(&models.AdminUser{}).Where("username = ?", username).Count(&n)
	if n > 0 {
		http.Redirect(w, r, "/admin/users?error=username+sudah+dipakai", http.StatusSeeOther)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "hash error", 500)
		return
	}
	u := models.AdminUser{Username: username, PassHash: string(hash), Role: role, Campus: campus, Active: true}
	if err := db.Conn().Create(&u).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	writeAudit(r, nil, "user.create", "user:"+username, "role="+role+" campus="+campus)
	http.Redirect(w, r, "/admin/users?ok=akun+"+username+"+dibuat", http.StatusSeeOther)
}

// POST /admin/users/{id}/password
func AdminUserPassword(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var u models.AdminUser
	if err := db.Conn().First(&u, chi.URLParam(r, "id")).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	pw := r.FormValue("password")
	if len(pw) < 8 {
		http.Redirect(w, r, "/admin/users?error=password+minimal+8+karakter", http.StatusSeeOther)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "hash error", 500)
		return
	}
	if err := db.Conn().Model(&u).Updates(map[string]any{
		"pass_hash":  string(hash),
		"updated_at": time.Now(),
	}).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	writeAudit(r, nil, "user.password_reset", "user:"+u.Username, "")
	http.Redirect(w, r, "/admin/users?ok=password+"+u.Username+"+diganti", http.StatusSeeOther)
}

// POST /admin/users/{id}/toggle
func AdminUserToggle(w http.ResponseWriter, r *http.Request) {
	var u models.AdminUser
	if err := db.Conn().First(&u, chi.URLParam(r, "id")).Error; err != nil {
		http.Error(w, "not found", 404)
		return
	}
	if cu := CurrentUser(r); cu != nil && cu.ID == u.ID {
		http.Redirect(w, r, "/admin/users?error=tidak+bisa+menonaktifkan+akun+sendiri", http.StatusSeeOther)
		return
	}
	// Never lock everyone out: keep at least one active admin standing.
	if u.Active && u.Role == models.RoleAdmin {
		var n int64
		db.Conn().Model(&models.AdminUser{}).
			Where("role = ? AND active = ? AND id <> ?", models.RoleAdmin, true, u.ID).Count(&n)
		if n == 0 {
			http.Redirect(w, r, "/admin/users?error=ini+admin+aktif+terakhir", http.StatusSeeOther)
			return
		}
	}
	newState := !u.Active
	if err := db.Conn().Model(&u).Update("active", newState).Error; err != nil {
		http.Error(w, "db error", 500)
		return
	}
	action := "user.deactivate"
	if newState {
		action = "user.activate"
	}
	writeAudit(r, nil, action, "user:"+u.Username, "")
	http.Redirect(w, r, "/admin/users?ok=status+"+u.Username+"+diubah", http.StatusSeeOther)
}
