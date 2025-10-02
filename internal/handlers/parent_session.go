package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
	svc "github.com/lojf/nextgen/internal/services"
)

const parentPhoneCookie = "parent_phone"
const parentNameCookie  = "parent_name"

func setParentCookies(w http.ResponseWriter, phone, name string) {
	phone = svc.NormPhone(phone)
	name = strings.TrimSpace(name)
	if phone != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     parentPhoneCookie,
			Value:    phone,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(30 * 24 * time.Hour),
		})
	}
	if name != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     parentNameCookie,
			Value:    name,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(30 * 24 * time.Hour),
		})
	}
}

func readParentCookies(r *http.Request) (phone, name string) {
	if c, err := r.Cookie(parentPhoneCookie); err == nil {
		phone = c.Value
	}
	if c, err := r.Cookie(parentNameCookie); err == nil {
		name = c.Value
	}
	return
}

func refreshParentCookiesIfNeeded(w http.ResponseWriter, r *http.Request, phone string) {
	if phone == "" { return }
	var p models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&p).Error; err == nil {
		setParentCookies(w, p.Phone, p.Name)
	}
}

func AccountLogout(w http.ResponseWriter, r *http.Request) {
	// Clear cookies
	http.SetCookie(w, &http.Cookie{Name: parentPhoneCookie, Value: "", Path: "/", Expires: time.Unix(0, 0)})
	http.SetCookie(w, &http.Cookie{Name: parentNameCookie,  Value: "", Path: "/", Expires: time.Unix(0, 0)})
	next := r.URL.Query().Get("next")
	if next == "" { next = "/account" }
	http.Redirect(w, r, next, http.StatusSeeOther)
}
func RequireParent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		phone, _ := readParentCookies(r)
		if strings.TrimSpace(phone) == "" {
			// Accept first-time pass-through if URL has ?phone=... and it exists in DB.
			qPhone := svc.NormPhone(r.URL.Query().Get("phone"))
			if qPhone != "" {
				var p models.Parent
				if err := db.Conn().Where("phone = ?", qPhone).First(&p).Error; err == nil {
					// Set cookies and continue to the page
					setParentCookies(w, p.Phone, p.Name)
					next.ServeHTTP(w, r)
					return
				}
			}
			// No cookie and no valid phone param: send to the appropriate phone gate
			gate := "/account"
			if strings.HasPrefix(r.URL.Path, "/my") {
				gate = "/my"
			}
			http.Redirect(w, r, gate, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}