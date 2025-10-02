package handlers

import (
	"net/http"
	"strings"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func AccountUnlinkTelegram(w http.ResponseWriter, r *http.Request) {
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
	// Clear links for this parent
	db.Conn().Model(&models.TelegramUser{}).
		Where("parent_id = ?", parent.ID).
		Updates(map[string]any{"parent_id": nil, "deliverable": false})

	http.Redirect(w, r, "/account/profile?success=unlinked", http.StatusSeeOther)
}
