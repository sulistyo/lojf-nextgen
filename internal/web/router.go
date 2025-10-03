package web

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/lojf/nextgen/internal/handlers"
)

func Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	tmpl := mustParseTemplates("templates")

	// Public pages
	r.Get("/", handlers.Home(tmpl))
	r.Get("/healthz", handlers.Health)
	r.Post("/tg/webhook", handlers.TelegramWebhook)
	r.Get("/switch-number", handlers.SwitchNumber)
	
	// --- Parent registration: phone-first flow ---
	r.Get("/register", handlers.RegisterPhoneForm(tmpl))
	r.Post("/register", handlers.RegisterPhoneSubmit)
	r.Get("/register/onboard", handlers.RegisterOnboardForm(tmpl))
	r.Post("/register/onboard", handlers.RegisterOnboardSubmit)
	r.Get("/register/kids", handlers.RegisterKidsForm(tmpl))
	r.Post("/register/kids", handlers.RegisterKidsSubmit)
	r.Get("/register/newchild", handlers.RegisterNewChildForm(tmpl))
	r.Post("/register/newchild", handlers.RegisterNewChildSubmit)

	// Class selection (continues the flow)
	r.Get("/register/classes", handlers.SelectClassForm(tmpl))
	r.Post("/register/classes", handlers.SelectClassSubmit(tmpl))

	// Parent self-service: cancel + “My registrations”
	r.Get("/cancel", handlers.CancelForm(tmpl))
	r.Post("/cancel", handlers.CancelSubmit(tmpl))
	r.Get("/my", handlers.MyPhoneForm(tmpl))
	r.With(handlers.RequireParent).Get("/my/list", handlers.MyList(tmpl))
	r.With(handlers.RequireParent).Get("/my/qr", handlers.MyQR(tmpl))

	// Parent Account
	r.Get("/account", handlers.AccountPhoneForm(tmpl)) // phone gate
	r.Get("/account/logout", handlers.AccountLogout)
	r.With(handlers.RequireParent).Get("/account/profile", handlers.AccountProfileForm(tmpl))
	r.With(handlers.RequireParent).Post("/account/profile", handlers.AccountProfileSubmit)
	r.With(handlers.RequireParent).Get("/account/children/new", handlers.AccountNewChildForm(tmpl))
	r.With(handlers.RequireParent).Post("/account/children/new", handlers.AccountNewChildSubmit)
	r.With(handlers.RequireParent).Get("/account/children/edit", handlers.AccountEditChildForm(tmpl))
	r.With(handlers.RequireParent).Post("/account/children/edit", handlers.AccountEditChildSubmit)
	r.With(handlers.RequireParent).Post("/account/children/delete", handlers.AccountDeleteChild)

	r.With(handlers.RequireParent).Post("/account/linkcode", handlers.AccountGenerateLinkCode)
	r.With(handlers.RequireParent).Post("/account/unlink_telegram", handlers.AccountUnlinkTelegram) 

	// QR and Check-in
	r.Get("/qr/{code}.png", handlers.QR)
	r.Get("/checkin", handlers.CheckinForm(tmpl))
	r.Post("/checkin", handlers.CheckinConfirm(tmpl))

	// --- Admin routes (with login + guard) ---
	r.Route("/admin", func(ar chi.Router) {
		// Auth endpoints (public)
		ar.Get("/login", handlers.AdminLoginForm(tmpl))
		ar.Post("/login", handlers.AdminLoginSubmit)
		ar.Post("/logout", handlers.AdminLogout)

		// Guarded admin pages
		ar.Group(func(ag chi.Router) {
			ag.Use(handlers.RequireAdmin)

			// Classes
			ag.Get("/classes", handlers.AdminClasses(tmpl))
			ag.Get("/classes/new", handlers.AdminNewClass(tmpl))
			ag.Post("/classes", handlers.AdminCreateClass)
			ag.Get("/classes/{id}/edit", handlers.AdminEditClassForm(tmpl))
			ag.Post("/classes/{id}", handlers.AdminUpdateClass)

			// Roster & Capacity
			ag.Get("/roster", handlers.AdminRoster(tmpl))
			ag.Get("/roster.csv", handlers.AdminRosterCSV)
			ag.Get("/capacity", handlers.AdminCapacity(tmpl))

			// Registration actions
			ag.Post("/registrations/{id}/checkin", handlers.AdminRegCheckin)
			ag.Post("/registrations/{id}/cancel", handlers.AdminRegCancel)
			ag.Post("/registrations/{id}/delete", handlers.AdminRegDelete)

			// Parents roster (moved here)
			ag.Get("/parents", handlers.AdminParentsList(tmpl))
			ag.Get("/parents/{id}", handlers.AdminParentShowForm(tmpl))
			ag.Post("/parents/{id}", handlers.AdminParentUpdate)
			ag.Post("/parents/{id}/children/update", handlers.AdminChildUpdate)
			ag.Post("/parents/{id}/children/delete", handlers.AdminChildDelete)
			ag.Post("/parents/{id}/delete", handlers.AdminParentDelete)

		})
	})

	return r
}
func mustParseTemplates(baseDir string) *template.Template {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil { loc = time.FixedZone("WIB", 7*3600) }

	funcs := template.FuncMap{
		"year":     func() string { return time.Now().Format("2006") },
		"jdate":    func(t time.Time) string { return t.In(loc).Format("Mon, 02 Jan 2006") },
		"jisodate": func(t time.Time) string { return t.In(loc).Format("2006-01-02") },
		// NEW: 12 January 2012
		"jlong":    func(t time.Time) string { return t.In(loc).Format("02 January 2006") },
	}

	p := template.New("").Funcs(funcs)
	p = template.Must(p.ParseGlob(filepath.Join(baseDir, "layouts", "*.tmpl")))
	p = template.Must(p.ParseGlob(filepath.Join(baseDir, "partials", "*.tmpl")))
	return p
}


