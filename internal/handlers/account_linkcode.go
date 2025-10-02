package handlers

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"
	"log"
	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

// generate 6-digit code using crypto/rand
func genCode6() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	n := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", n)
}

func AccountGenerateLinkCode(w http.ResponseWriter, r *http.Request) {
	// ensure the caller is a logged-in parent (via your RequireParent middleware)
	phone, _ := readParentCookies(r)
	if strings.TrimSpace(phone) == "" {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	var parent models.Parent
	if err := db.Conn().Where("phone = ?", phone).First(&parent).Error; err != nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	// housekeeping: remove long-expired/used codes for this parent (optional)
	_ = db.Conn().Where("parent_id = ? AND (used_at IS NOT NULL OR expires_at < ?)", parent.ID, time.Now().Add(-24*time.Hour)).Delete(&models.LinkCode{}).Error

	// try up to 10 times to avoid unique collisions
	var code string
	for i := 0; i < 10; i++ {
		code = genCode6()
		lc := models.LinkCode{
			Code:      code,
			ParentID:  parent.ID,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		if err := db.Conn().Create(&lc).Error; err != nil {
			log.Printf("linkcode create error: %v", err) 
			// if unique collision, retry; otherwise bubble up
			if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
				!strings.Contains(strings.ToLower(err.Error()), "constraint") {
				http.Error(w, "db error", http.StatusInternalServerError)
				return
			}
			continue
		}
		// success
		http.Redirect(w, r, "/account/profile?link_code="+code, http.StatusSeeOther)
		return
	}

	http.Error(w, "unable to generate link code, please try again", http.StatusInternalServerError)
}
