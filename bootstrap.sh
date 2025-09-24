#!/usr/bin/env bash
# Bootstrap folder structure & starter files (use if you DIDN'T upload the zip).
# Usage:
#   curl -fsSL https://example.com/lojf-bootstrap.sh | bash
# or copy & paste into /root/apps/lojf and run: bash bootstrap.sh

set -euo pipefail

APP_DIR="${APP_DIR:-/root/apps/lojf}"
echo "Creating structure under $APP_DIR"
mkdir -p "$APP_DIR"
cd "$APP_DIR"

# Initialize minimal scaffold if not exists
if [ ! -f go.mod ]; then
  cat > go.mod <<'EOF'
module github.com/lojf/nextgen

go 1.22.0

require (
	github.com/go-chi/chi/v5 v5.0.11
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.25.11
)
EOF
fi

mkdir -p cmd/server internal/{web,handlers,models,db} templates/{layouts,partials,pages/{parents,admin}} deploy bin

# main.go
cat > cmd/server/main.go <<'EOF'
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/web"
)

func main() {
	if err := db.Init(); err != nil {
		log.Fatalf("db init: %v", err)
	}
	r := web.Router()
	addr := getEnv("ADDR", ":8080")
	log.Printf("LOJF NextGen listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
func getEnv(key, def string) string { if v := os.Getenv(key); v != "" { return v }; return def }
EOF

# router.go
cat > internal/web/router.go <<'EOF'
package web
import (
	"html/template"
	"net/http"
	"path/filepath"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lojf/nextgen/internal/handlers"
)
func Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	tmpl := mustParseTemplates("templates")
	r.Get("/", handlers.Home(tmpl))
	r.Get("/healthz", handlers.Health)
	r.Get("/register", handlers.RegisterForm(tmpl))
	r.Post("/register", handlers.RegisterSubmit(tmpl))
	r.Get("/admin/classes", handlers.AdminClasses(tmpl))
	return r
}
func mustParseTemplates(baseDir string) *template.Template {
	p := template.New("").Funcs(template.FuncMap{})
	p = template.Must(p.ParseGlob(filepath.Join(baseDir, "layouts", "*.tmpl")))
	p = template.Must(p.ParseGlob(filepath.Join(baseDir, "partials", "*.tmpl")))
	p = template.Must(p.ParseGlob(filepath.Join(baseDir, "pages", "**", "*.tmpl")))
	return p
}
EOF

# handlers
cat > internal/handlers/home.go <<'EOF'
package handlers
import ("html/template"; "net/http")
func Home(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := t.ExecuteTemplate(w, "home.tmpl", map[string]any{"Title":"LOJF NextGen Manager"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
EOF
cat > internal/handlers/health.go <<'EOF'
package handlers
import ("encoding/json"; "net/http")
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type","application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok":true,"svc":"lojf-nextgen"})
}
EOF
cat > internal/handlers/register.go <<'EOF'
package handlers
import ("html/template"; "net/http"; "time"; "github.com/lojf/nextgen/internal/db"; "github.com/lojf/nextgen/internal/models")
func RegisterForm(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := t.ExecuteTemplate(w, "parents/register.tmpl", map[string]any{"Title":"Register Parent & Child"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
func RegisterSubmit(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
		parentName := r.FormValue("parent_name"); childName := r.FormValue("child_name"); dob := r.FormValue("child_dob")
		if parentName==""||childName==""||dob=="" { http.Error(w, "missing fields", http.StatusBadRequest); return }
		d, err := time.Parse("2006-01-02", dob); if err != nil { http.Error(w, "invalid date", http.StatusBadRequest); return }
		var parent models.Parent
		res := db.Conn().Where("name = ?", parentName).First(&parent)
		if res.Error != nil || parent.ID==0 { parent = models.Parent{Name: parentName}; if err := db.Conn().Create(&parent).Error; err != nil { http.Error(w,"failed to save parent",500); return } }
		child := models.Child{Name: childName, BirthDate: d, ParentID: parent.ID}
		if err := db.Conn().Create(&child).Error; err != nil { http.Error(w,"failed to save child",500); return }
		if err := t.ExecuteTemplate(w, "parents/registered.tmpl", map[string]any{"Title":"Registered","ParentName":parentName,"ChildName":childName}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
EOF
cat > internal/handlers/admin.go <<'EOF'
package handlers
import ("html/template"; "net/http"; "github.com/lojf/nextgen/internal/db"; "github.com/lojf/nextgen/internal/models")
func AdminClasses(t *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var classes []models.Class
		if err := db.Conn().Find(&classes).Error; err != nil { http.Error(w, "db error", 500); return }
		if err := t.ExecuteTemplate(w, "admin/classes.tmpl", map[string]any{"Title":"Admin • Classes","Classes":classes}); err != nil {
			http.Error(w, err.Error(), 500)
		}
	}
}
EOF

# models & db
cat > internal/models/models.go <<'EOF'
package models
import "time"
type Parent struct { ID uint `gorm:"primaryKey"`; CreatedAt time.Time; UpdatedAt time.Time; Name string; Phone string; Email string; Children []Child }
type Child struct { ID uint `gorm:"primaryKey"`; CreatedAt time.Time; UpdatedAt time.Time; Name string; BirthDate time.Time; ParentID uint; Parent Parent }
type Class struct { ID uint `gorm:"primaryKey"`; CreatedAt time.Time; UpdatedAt time.Time; Date time.Time; Name string; Capacity int }
type Registration struct { ID uint `gorm:"primaryKey"`; CreatedAt time.Time; UpdatedAt time.Time; ParentID uint; ChildID uint; ClassID uint; Status string; Code string }
EOF
cat > internal/db/db.go <<'EOF'
package db
import ("gorm.io/driver/sqlite"; "gorm.io/gorm"; "github.com/lojf/nextgen/internal/models"; "log")
var conn *gorm.DB
func Init() error {
	var err error
	conn, err = gorm.Open(sqlite.Open("nextgen.db"), &gorm.Config{})
	if err != nil { return err }
	if err := conn.AutoMigrate(&models.Parent{}, &models.Child{}, &models.Class{}, &models.Registration{}); err != nil { return err }
	log.Println("database ready (sqlite)")
	return nil
}
func Conn() *gorm.DB { return conn }
EOF

# templates
cat > templates/layouts/base.tmpl <<'EOF'
{{define "base"}}
<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<script src="https://cdn.tailwindcss.com"></script>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/flowbite/2.5.2/flowbite.min.css" />
<script defer src="https://cdnjs.cloudflare.com/ajax/libs/flowbite/2.5.2/flowbite.min.js"></script>
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
</head><body class="bg-gray-50 text-gray-900">
{{template "navbar" .}}
<main class="max-w-6xl mx-auto p-6">{{template "content" .}}</main>
{{template "footer" .}}
</body></html>
{{end}}
EOF
cat > templates/partials/navbar.tmpl <<'EOF'
{{define "navbar"}}
<nav class="bg-white border-b"><div class="max-w-6xl mx-auto px-4">
<div class="flex justify-between items-center h-14">
<a href="/" class="font-semibold">LOJF NextGen</a>
<div class="space-x-4">
<a class="text-sm hover:underline" href="/register">Register</a>
<a class="text-sm hover:underline" href="/admin/classes">Admin</a>
<a class="text-sm hover:underline" href="/healthz">Health</a>
</div></div></div></nav>
{{end}}
EOF
cat > templates/partials/footer.tmpl <<'EOF'
{{define "footer"}}
<footer class="mt-12 border-t py-6 text-center text-sm text-gray-500">© LOJF • Built for Kids Ministry Ops</footer>
{{end}}
EOF
cat > templates/pages/home.tmpl <<'EOF'
{{define "content"}}
<section class="grid gap-6 md:grid-cols-2 items-center">
  <div>
    <h1 class="text-3xl font-bold mb-3">LOJF NextGen Manager</h1>
    <p class="text-gray-600 mb-6">A clean, scalable starting point.</p>
    <div class="flex gap-3">
      <a href="/register" class="px-4 py-2 rounded-xl bg-gray-900 text-white">Register Parent & Child</a>
      <a href="/admin/classes" class="px-4 py-2 rounded-xl border">Admin • Classes</a>
    </div>
  </div>
  <div class="bg-white border rounded-2xl p-6 shadow-sm">
    <h2 class="font-semibold mb-2">What’s inside</h2>
    <ul class="text-sm list-disc pl-5 space-y-1 text-gray-700">
      <li>Go (chi) + GORM (SQLite)</li>
      <li>Tailwind + Flowbite UI</li>
      <li>Templates layout/partials</li>
      <li>Health check endpoint</li>
    </ul>
  </div>
</section>
{{end}}
{{template "base" .}}
EOF
cat > templates/pages/parents/register.tmpl <<'EOF'
{{define "content"}}
<h1 class="text-2xl font-bold mb-4">Register Parent & Child</h1>
<form method="POST" class="grid gap-4 max-w-lg bg-white p-6 rounded-2xl border">
  <div><label class="block text-sm mb-1">Parent Name</label><input name="parent_name" class="w-full rounded-xl border p-2" required></div>
  <div><label class="block text-sm mb-1">Child Name</label><input name="child_name" class="w-full rounded-xl border p-2" required></div>
  <div><label class="block text-sm mb-1">Child Date of Birth</label><input type="date" name="child_dob" class="w-full rounded-xl border p-2" required></div>
  <button class="px-4 py-2 rounded-xl bg-gray-900 text-white">Save</button>
</form>
{{end}}
{{template "base" .}}
EOF
cat > templates/pages/parents/registered.tmpl <<'EOF'
{{define "content"}}
<div class="max-w-lg bg-white p-6 rounded-2xl border">
  <h1 class="text-2xl font-semibold mb-2">Registration Saved</h1>
  <p class="text-gray-700 mb-4">Saved successfully.</p>
  <a class="text-sm underline" href="/register">Add another</a>
</div>
{{end}}
{{template "base" .}}
EOF
cat > templates/pages/admin/classes.tmpl <<'EOF'
{{define "content"}}
<h1 class="text-2xl font-bold mb-4">Admin • Classes</h1>
<div class="bg-white border rounded-2xl p-6">
  <p class="text-gray-600">No classes yet.</p>
</div>
{{end}}
{{template "base" .}}
EOF

# vhost and service
cat > deploy/apache-nextgen.conf <<'EOF'
<VirtualHost *:80>
    ServerName nextgen.lojf.id
    ServerAdmin webmaster@lojf.id
    ProxyPreserveHost On
    ProxyRequests Off
    ProxyPass / http://127.0.0.1:8080/
    ProxyPassReverse / http://127.0.0.1:8080/
    ErrorLog ${APACHE_LOG_DIR}/nextgen-error.log
    CustomLog ${APACHE_LOG_DIR}/nextgen-access.log combined
</VirtualHost>
EOF
cat > deploy/lojf-nextgen.service <<'EOF'
[Unit]
Description=LOJF NextGen Manager
After=network.target
[Service]
WorkingDirectory=/root/apps/lojf
ExecStart=/root/apps/lojf/bin/lojf-nextgen
Restart=always
RestartSec=3
Environment=ADDR=:8080
User=root
Group=root
[Install]
WantedBy=multi-user.target
EOF

# Makefile
cat > Makefile <<'EOF'
APP=lojf-nextgen
PKG=./cmd/server
.PHONY: all tidy build run test clean fmt
all: build
tidy: ; go mod tidy
fmt: ; go fmt ./...
build: tidy ; CGO_ENABLED=1 go build -o bin/$(APP) $(PKG)
run: ; ./bin/$(APP)
test: ; go test ./... -v
clean: ; rm -rf bin
EOF

echo "Done. Run: go mod tidy && make build && ./bin/lojf-nextgen"
